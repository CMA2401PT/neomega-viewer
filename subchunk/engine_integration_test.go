package subchunk_test

import (
	"os"
	"testing"

	"celler"
	"celler/layer_storage"
	"neomega-blocks/ir"
	"neomega-viewer/blockmesh"
	"neomega-viewer/demo"
	"neomega-viewer/meshing"
	"neomega-viewer/subchunk"
)

func realEngine(t *testing.T) *subchunk.Engine {
	t.Helper()
	dir := os.Getenv("NEOMEGA_ASSETS_DIR")
	if dir == "" {
		t.Skip("set NEOMEGA_ASSETS_DIR to verify actual palette/model integration")
	}
	parser, err := blockmesh.Open(blockmesh.OpenOptions{AssetsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	return subchunk.New(parser)
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
		result, err := engine.Build(solid, subchunk.Options{Meshing: check.Options})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Mesh.Quads) != check.Quads || result.Stats.Blocks != 4096 || result.Stats.InputQuads != 24576 {
			t.Fatalf("default-filled solid lost cells: %+v", result.Stats)
		}
	}
	for _, scene := range demo.List() {
		t.Run(scene.ID, func(t *testing.T) {
			layer, err := demo.Build(scene.ID)
			if err != nil {
				t.Fatal(err)
			}
			before := subchunk.EncodeIDs(layer)
			result, err := engine.Build(layer, subchunk.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(subchunk.EncodeIDs(layer)) {
				t.Fatal("rendering mutated caller palette")
			}
			if scene.ID == "empty" && result.Stats.OutputQuads != 0 {
				t.Fatal("air has visible faces")
			}
			if scene.ID == "slabs" && result.Stats.ClippedQuads == 0 {
				t.Fatal("real slab/stone interface was not clipped")
			}
			t.Logf("%s: %+v", scene.ID, result.Stats)
		})
	}
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
		layer.Set(celler.ToSubChunkBlockPos(4, 4, 4), id(t, check.A))
		layer.Set(celler.ToSubChunkBlockPos(5, 4, 4), id(t, check.B))
		result, err := engine.Build(layer, subchunk.Options{Meshing: meshing.Options{DisableGreedy: true}})
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
			a, b := celler.ToSubChunkBlockPos(4, 4, 4), celler.ToSubChunkBlockPos(5, 4, 4)
			layer.Set(a, isolated)
			layer.Set(b, isolated)
			stored, err := engine.Build(layer, subchunk.Options{})
			if err != nil {
				t.Fatal(err)
			}
			layer.Set(a, east)
			layer.Set(b, west)
			connected, err := engine.Build(layer, subchunk.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if connected.Stats.InputQuads <= stored.Stats.InputQuads {
				t.Fatal("Java connection states did not add multipart geometry")
			}
			layer.Set(b, air)
			single, err := engine.Build(layer, subchunk.Options{})
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
			layer.Set(celler.ToSubChunkBlockPos(4, 4, 4), fluid)
			single, err := engine.Build(layer, subchunk.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if single.Mesh.Buffers().Bounds.Max[1] != 4+8.0/9 {
				t.Fatal("isolated fluid height changed")
			}
			layer.Set(celler.ToSubChunkBlockPos(4, 5, 4), fluid)
			stacked, err := engine.Build(layer, subchunk.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if stacked.Stats.RemovedQuads != 2 {
				t.Fatalf("stacked fluid did not fill the lower cell: %+v", stacked.Stats)
			}
		})
	}
}
