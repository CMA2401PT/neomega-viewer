package subchunk_test

import (
	"math"
	"slices"
	"testing"

	"celler/layer_storage"
	"neomega-blocks/ir"
	"neomega-viewer-core/viewer/cube"
	"neomega-viewer-core/viewer/mesher"
	"neomega-viewer-core/viewer/mesher/meshing"
	blockmesh "neomega-viewer-core/viewer/resources"
	"neomega-viewer-core/viewer/splitlayer"
	"neomega-viewer/demo/scenes"
	"neomega-viewer/demo/subchunk"
)

func realEngine(t *testing.T) *mesher.Mesher[uint32] {
	t.Helper()
	parser, err := blockmesh.Open(blockmesh.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return mesher.NewMesher(cube.New(16), parser.Tables)
}
func id(t *testing.T, java string) uint32 {
	t.Helper()
	return ir.IRBlock(java).Tidy().Hash()
}

func TestFilledDefaultAndEveryDemo(t *testing.T) {
	engine := realEngine(t)
	solid, err := demo.Build("solid")
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		Options meshing.Options
		Quads   int
	}{
		{meshing.Options{}, 6}, {meshing.Options{DisableGreedy: true}, 1536}, {meshing.Options{DisableCulling: true, DisableGreedy: true}, 24576},
	} {
		result, err := engine.MeshLayer(plain(solid).Data, mesher.Options{Meshing: check.Options})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Mesh.Quads) != check.Quads || result.Stats.Blocks != 4096 || result.Stats.InputQuads != 24576 {
			t.Fatalf("default-filled solid lost cells: %+v", result.Stats)
		}
		removed := 23040
		if check.Options.DisableCulling {
			removed = 0
		}
		if result.Stats.RemovedQuads != removed || result.Stats.ClippedQuads != 0 {
			t.Fatalf("solid culling changed: %+v", result.Stats)
		}
	}
	layerSplitter := splitlayer.New()
	for _, scene := range demo.List() {
		t.Run(scene.ID, func(t *testing.T) {
			layer, err := demo.Build(scene.ID)
			if err != nil {
				t.Fatal(err)
			}
			before := subchunk.EncodeIDs(layer)
			input := plain(layer)
			original := slices.Clone(input.Data)
			result, err := engine.MeshLayers(layerSplitter.SplitBlocks(input.Data), mesher.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(subchunk.EncodeIDs(layer)) {
				t.Fatal("rendering mutated caller palette")
			}
			if !slices.Equal(original, input.Data) {
				t.Fatal("rendering mutated caller plain layer")
			}
			if scene.ID == "empty" && result.Stats.OutputQuads != 0 {
				t.Fatal("air has visible faces")
			}
			if scene.ID == "slabs" && (result.Stats.RemovedQuads != 814 || result.Stats.ClippedQuads != 6 || result.Stats.OutputQuads != 31) {
				t.Fatalf("real slab/stone clipping changed: %+v", result.Stats)
			}
			t.Logf("%s: %+v", scene.ID, result.Stats)
		})
	}
}

func TestWaterCubeExterior(t *testing.T) {
	layer, err := demo.Build("water_cube")
	if err != nil {
		t.Fatal(err)
	}
	result, err := realEngine(t).MeshLayers(
		splitlayer.New().SplitBlocks(plain(layer).Data),
		mesher.Options{Meshing: meshing.Options{DisableGreedy: true}},
	)
	if err != nil {
		t.Fatal(err)
	}
	// 只统计左侧水块，排除石头地板、中央水柱和右侧玻璃缸。
	tops, sides, bottoms := 0, 0, 0
	for _, quad := range result.Mesh.Quads {
		if quad.Surface.Texture != "minecraft:block/water_still" && quad.Surface.Texture != "minecraft:block/water_flow" {
			continue
		}
		inside := true
		for _, p := range quad.Positions {
			inside = inside && p[0] >= 2 && p[0] <= 6 && p[2] >= 2 && p[2] <= 6
		}
		if !inside {
			continue
		}
		switch quad.Normal[1] {
		case 1:
			tops++
			for _, p := range quad.Positions {
				if math.Abs(p[1]-(5-1.0/9)) > 1e-9 {
					t.Fatalf("water cube top Y = %g, want %g", p[1], 5-1.0/9)
				}
			}
		case -1:
			bottoms++
		default:
			sides++
		}
	}
	if tops != 16 || sides != 64 || bottoms != 0 {
		t.Fatalf("water cube exterior: top=%d sides=%d bottom=%d total=%d, want 16+64+0=80", tops, sides, bottoms, tops+sides+bottoms)
	}
	t.Logf("water_cube DisableGreedy: %+v; left cube: 16 tops + 64 sides = 80", result.Stats)
}

func TestTransparentInterfaces(t *testing.T) {
	engine := realEngine(t)
	for _, check := range []struct {
		A, B    string
		Removed int
	}{
		{"minecraft:glass []", "minecraft:glass []", 2},
		{"minecraft:glass []", "minecraft:red_stained_glass []", 0},
		{"minecraft:glass []", "minecraft:stone []", 1},
		{`minecraft:oak_leaves ["distance"="7","persistent"="false"]`, "minecraft:stone []", 1},
	} {
		layer := layer_storage.MakePaletteStorage(id(t, "minecraft:air []"))
		layer.Set(layer_storage.At(4, 4, 4), id(t, check.A))
		layer.Set(layer_storage.At(5, 4, 4), id(t, check.B))
		result, err := engine.MeshLayer(plain(layer).Data, mesher.Options{Meshing: meshing.Options{DisableGreedy: true}})
		if err != nil {
			t.Fatal(err)
		}
		if result.Stats.RemovedQuads != check.Removed {
			t.Fatalf("%s/%s: %+v", check.A, check.B, result.Stats)
		}
	}
}

func TestConnectionsComeFromJavaHash(t *testing.T) {
	engine := realEngine(t)
	for _, name := range []string{"oak_fence", "glass_pane"} {
		t.Run(name, func(t *testing.T) {
			air := id(t, "minecraft:air []")
			isolated := id(t, "minecraft:"+name+` ["east"="false","north"="false","south"="false","west"="false"]`)
			east := id(t, "minecraft:"+name+` ["east"="true","north"="false","south"="false","west"="false"]`)
			west := id(t, "minecraft:"+name+` ["east"="false","north"="false","south"="false","west"="true"]`)
			layer := layer_storage.MakePaletteStorage(air)
			a, b := layer_storage.At(4, 4, 4), layer_storage.At(5, 4, 4)
			layer.Set(a, isolated)
			layer.Set(b, isolated)
			stored, err := engine.MeshLayer(plain(layer).Data, mesher.Options{})
			if err != nil {
				t.Fatal(err)
			}
			layer.Set(a, east)
			layer.Set(b, west)
			connected, err := engine.MeshLayer(plain(layer).Data, mesher.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if connected.Stats.InputQuads <= stored.Stats.InputQuads {
				t.Fatal("Java connection states did not add multipart geometry")
			}
			layer.Set(b, air)
			single, err := engine.MeshLayer(plain(layer).Data, mesher.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if single.Stats.InputQuads*2 != connected.Stats.InputQuads {
				t.Fatal("removing a neighbor changed the stored connection")
			}
		})
	}
}

func TestFluidHeightWithJavaHashes(t *testing.T) {
	engine := realEngine(t)
	for _, name := range []string{"water", "lava"} {
		t.Run(name, func(t *testing.T) {
			layer := layer_storage.MakePaletteStorage(id(t, "minecraft:air []"))
			fluid := id(t, "minecraft:"+name+` ["level"="0"]`)
			layer.Set(layer_storage.At(4, 4, 4), fluid)
			single, err := engine.MeshLayer(plain(layer).Data, mesher.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if single.Mesh.Buffers().Bounds.Max[1] != 4+8.0/9 {
				t.Fatal("isolated fluid height changed")
			}
			layer.Set(layer_storage.At(4, 5, 4), fluid)
			stacked, err := engine.MeshLayer(plain(layer).Data, mesher.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if stacked.Stats.RemovedQuads != 2 {
				t.Fatalf("stacked fluid did not fill the lower cell: %+v", stacked.Stats)
			}
		})
	}
}

func TestFluidHeightUsesOnlySameFamilyAbove(t *testing.T) {
	engine := realEngine(t)
	for _, check := range []struct {
		name, base, above string
		y                 int
		height            float64
	}{
		{"flowing isolated", `water ["level"="7"]`, "air []", 4, 1.0 / 9},
		{"different level", `water ["level"="7"]`, `water ["level"="2"]`, 4, 1},
		{"different family", `water ["level"="7"]`, `lava ["level"="0"]`, 4, 1.0 / 9},
		{"falling isolated", `water ["level"="8"]`, "air []", 4, 1},
		{"section top", `water ["level"="0"]`, "air []", 15, 8.0 / 9},
	} {
		t.Run(check.name, func(t *testing.T) {
			layer := layer_storage.MakePlainStorage(id(t, "minecraft:air []"))
			layer.Set(layer_storage.At(4, check.y, 4), id(t, "minecraft:"+check.base))
			if check.y < 15 {
				layer.Set(layer_storage.At(4, check.y+1, 4), id(t, "minecraft:"+check.above))
			}
			result, err := engine.MeshLayer(layer.Data, mesher.Options{Meshing: meshing.Options{DisableCulling: true, DisableGreedy: true}})
			if err != nil {
				t.Fatal(err)
			}
			for _, quad := range result.Mesh.Quads[:6] {
				if quad.Normal[1] == 1 && quad.Positions[0][1] != float64(check.y)+check.height {
					t.Fatalf("lower fluid height = %g, want %g", quad.Positions[0][1], float64(check.y)+check.height)
				}
			}
		})
	}
}

func plain(layer *layer_storage.Palette[uint32]) *layer_storage.Plain[uint32] {
	result := &layer_storage.Plain[uint32]{}
	result.CopyFrom(layer)
	return result
}
