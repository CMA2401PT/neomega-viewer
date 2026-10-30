package mesher_test

import (
	"math"
	"strings"
	"testing"

	"neomega-blocks/ir"
	"neomega-viewer/demo/blockmesh"
	"neomega-viewer/mesher"
	"neomega-viewer/mesher/geometry"
	"neomega-viewer/mesher/material"
	"neomega-viewer/mesher/meshing"
	"neomega-viewer/splitlayer"
)

func multiLayerEngine(t *testing.T) *mesher.Mesher[uint32] {
	t.Helper()
	parser, err := blockmesh.Open(blockmesh.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return mesher.NewMesher(parser.Tables())
}

func hash(block ir.IRBlock) uint32 { return block.Tidy().Hash() }

func cell(x, y, z int) int { return (x << 8) | (z << 4) | y }

func airSubchunk() []uint32 {
	blocks := make([]uint32, 4096)
	for i := range blocks {
		blocks[i] = hash("minecraft:air []")
	}
	return blocks
}

func TestWaterloggedSlabCoplanarClipping(t *testing.T) {
	engine := multiLayerEngine(t)
	blocks := airSubchunk()
	blocks[cell(4, 4, 4)] = hash(`minecraft:oak_slab ["type"="bottom","waterlogged"="true"]`)
	layers := splitlayer.New().SplitSubchunk(blocks)
	result, err := engine.MeshMultiLayerSubchunks(layers, mesher.Options{
		Meshing: meshing.Options{DisableGreedy: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := mesher.Stats{
		Stats:  meshing.Stats{InputQuads: 12, RemovedQuads: 1, ClippedQuads: 4, VisibleQuads: 11, OutputQuads: 11},
		Blocks: 1,
		Shapes: 2,
	}
	if result.Stats != want {
		t.Fatalf("isolated wet slab: %+v, want %+v", result.Stats, want)
	}
	waterTop, waterSides, slabFaces := 0, 0, 0
	for _, quad := range result.Mesh.Quads {
		if quad.Surface.Mode != material.Blend {
			slabFaces++
			continue
		}
		switch quad.Normal[1] {
		case -1:
			t.Fatal("coplanar water bottom survived")
		case 1:
			waterTop++
			assertYRange(t, quad, 4+8.0/9, 4+8.0/9)
		default:
			waterSides++
			assertYRange(t, quad, 4.5, 4+8.0/9)
		}
	}
	if waterTop != 1 || waterSides != 4 || slabFaces != 6 {
		t.Fatalf("top=%d sides=%d slab=%d", waterTop, waterSides, slabFaces)
	}
	raw, err := engine.MeshMultiLayerSubchunks(layers, mesher.Options{
		Meshing: meshing.Options{DisableCulling: true, DisableGreedy: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if raw.Stats.RemovedQuads != 0 || raw.Stats.ClippedQuads != 0 || raw.Stats.OutputQuads != 12 {
		t.Fatalf("culling toggle: %+v", raw.Stats)
	}
}

func TestPoolCullsWaterAcrossLayers(t *testing.T) {
	engine := multiLayerEngine(t)
	blocks := airSubchunk()
	for x := 2; x < 7; x++ {
		for z := 2; z < 7; z++ {
			blocks[cell(x, 3, z)] = hash("minecraft:stone []")
			if x == 2 || x == 6 || z == 2 || z == 6 {
				blocks[cell(x, 4, z)] = hash("minecraft:stone []")
			} else {
				blocks[cell(x, 4, z)] = hash(`minecraft:water ["level"="0"]`)
			}
		}
	}
	blocks[cell(4, 4, 4)] = hash(`minecraft:oak_slab ["type"="bottom","waterlogged"="true"]`)
	layers := splitlayer.New().SplitSubchunk(blocks)
	result, err := engine.MeshMultiLayerSubchunks(layers, mesher.Options{
		Meshing: meshing.Options{DisableGreedy: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 中心水侧面先被台阶裁半，再被邻水全删，只应计入 RemovedQuads。
	if result.Stats.InputQuads != 306 || result.Stats.RemovedQuads != 191 || result.Stats.ClippedQuads != 0 || result.Stats.VisibleQuads != 115 {
		t.Fatalf("pool stats: %+v", result.Stats)
	}
	waterTops := 0
	for _, quad := range result.Mesh.Quads {
		if quad.Surface.Mode == material.Blend {
			if quad.Normal != (geometry.Vec3{0, 1, 0}) {
				t.Fatal("internal or stone-covered water face survived")
			}
			waterTops++
		}
	}
	if waterTops != 9 {
		t.Fatalf("pool has %d water tops, want 9", waterTops)
	}
}

func TestFluidHeightAcrossLayersPreservesSlab(t *testing.T) {
	engine := multiLayerEngine(t)
	for _, wetBelow := range []bool{true, false} {
		blocks := airSubchunk()
		water := hash(`minecraft:water ["level"="0"]`)
		wetSlab := hash(`minecraft:oak_slab ["type"="bottom","waterlogged"="true"]`)
		blocks[cell(4, 4, 4)], blocks[cell(4, 5, 4)] = water, wetSlab
		if wetBelow {
			blocks[cell(4, 4, 4)], blocks[cell(4, 5, 4)] = wetSlab, water
		}
		layers := splitlayer.New().SplitSubchunk(blocks)
		raw, err := engine.MeshMultiLayerSubchunks(layers, mesher.Options{
			Meshing: meshing.Options{DisableCulling: true, DisableGreedy: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		if raw.Stats.Blocks != 2 || raw.Stats.Shapes != 3 || raw.Stats.InputQuads != 18 {
			t.Fatalf("wetBelow=%t: lost fluid or slab: %+v", wetBelow, raw.Stats)
		}
		lowerTop, slabFaces := 0, 0
		for _, quad := range raw.Mesh.Quads {
			if quad.Surface.Mode != material.Blend {
				slabFaces++
			} else if quad.Normal[1] == 1 && quad.Positions[0][1] < 5.5 {
				lowerTop++
				assertYRange(t, quad, 5, 5)
			}
		}
		if lowerTop != 1 || slabFaces != 6 {
			t.Fatalf("wetBelow=%t: lowerTop=%d slabFaces=%d", wetBelow, lowerTop, slabFaces)
		}
		culled, err := engine.MeshMultiLayerSubchunks(layers, mesher.Options{
			Meshing: meshing.Options{DisableGreedy: true},
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, quad := range culled.Mesh.Quads {
			if quad.Surface.Mode == material.Blend && quad.Normal[1] != 0 && quad.Positions[0][1] == 5 {
				t.Fatalf("wetBelow=%t: internal horizontal water face survived", wetBelow)
			}
		}
	}
}

func TestMultiLayerErrorsIncludeLayer(t *testing.T) {
	engine := mesher.NewMesher(&mesher.Tables[uint32]{Index: map[uint32][]mesher.ModelID{0: {}}})
	layers := [][]uint32{make([]uint32, 4096), make([]uint32, 4096)}
	layers[1][cell(2, 3, 4)] = 7
	_, err := engine.MeshMultiLayerSubchunks(layers, mesher.Options{})
	if err == nil || !strings.Contains(err.Error(), "layer 1, cell (2,3,4), ID 7") {
		t.Fatalf("missing geometry error lost layer or position: %v", err)
	}
	layers[1] = layers[1][:4095]
	_, err = engine.MeshMultiLayerSubchunks(layers, mesher.Options{})
	if err == nil || !strings.Contains(err.Error(), "layer 1:") {
		t.Fatalf("length error lost layer: %v", err)
	}
}

func assertYRange(t *testing.T, quad geometry.Quad, wantMin, wantMax float64) {
	t.Helper()
	lo, hi := quad.Positions[0][1], quad.Positions[0][1]
	for _, p := range quad.Positions {
		lo, hi = min(lo, p[1]), max(hi, p[1])
	}
	if math.Abs(lo-wantMin) > 1e-9 || math.Abs(hi-wantMax) > 1e-9 {
		t.Fatalf("Y range = [%g,%g], want [%g,%g]", lo, hi, wantMin, wantMax)
	}
}
