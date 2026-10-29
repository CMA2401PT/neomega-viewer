package subchunk_test

import (
	"os"
	"testing"

	"celler"
	"celler/layer_storage"
	"neomega-blocks/ir"
	blocks "neomega-blocks/presets/block-converts/v1"
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
	block, ok := blocks.ConvertFromJava(ir.IRBlock(java))
	if !ok {
		t.Fatal("unknown test state", java)
	}
	return block.Hash()
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

func TestTransparentInterfacesAndInferredConnections(t *testing.T) {
	engine := realEngine(t)
	for _, check := range []struct {
		A, B    string
		Removed int
	}{
		{"glass", "glass", 2}, {"glass", "red_stained_glass", 0}, {"glass", "stone", 1}, {"oak_leaves", "stone", 1},
	} {
		layer := layer_storage.MakePaletteStorage(id(t, "minecraft:air []"))
		layer.Set(celler.ToSubChunkBlockPos(4, 4, 4), id(t, "minecraft:"+check.A+" []"))
		layer.Set(celler.ToSubChunkBlockPos(5, 4, 4), id(t, "minecraft:"+check.B+" []"))
		result, err := engine.Build(layer, subchunk.Options{Meshing: meshing.Options{DisableGreedy: true}})
		if err != nil {
			t.Fatal(err)
		}
		if result.Stats.RemovedQuads != check.Removed {
			t.Fatalf("%s/%s: %+v", check.A, check.B, result.Stats)
		}
	}
	layer := layer_storage.MakePaletteStorage(id(t, "minecraft:air []"))
	fence := id(t, "minecraft:oak_fence []")
	layer.Set(celler.ToSubChunkBlockPos(4, 4, 4), fence)
	layer.Set(celler.ToSubChunkBlockPos(5, 4, 4), fence)
	connected, err := engine.Build(layer, subchunk.Options{})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := engine.Build(layer, subchunk.Options{KeepConnections: true})
	if err != nil {
		t.Fatal(err)
	}
	if connected.Stats.InputQuads <= stored.Stats.InputQuads {
		t.Fatal("fence links were not inferred")
	}
}
