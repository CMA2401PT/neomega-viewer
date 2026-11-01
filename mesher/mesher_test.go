package mesher

import (
	"bytes"
	"math"
	"reflect"
	"testing"

	blockgeometry "neomega-blocks/geometry"
	"neomega-blocks/geometry/material"
	"neomega-blocks/ir"
	preset "neomega-blocks/presets/java/v1/geometry"
	"neomega-viewer/mesher/geometry"
	"neomega-viewer/mesher/meshing"
)

type custom32 uint32
type custom64 uint64

func TestNumericTablesAndMeshing(t *testing.T) {
	t.Run("uint32", func(t *testing.T) { checkNumericTables[uint32](t, 0xfedcba98) })
	t.Run("uint64", func(t *testing.T) { checkNumericTables[uint64](t, 1<<60+7) })
	t.Run("named32", func(t *testing.T) { checkNumericTables[custom32](t, 0xfedcba98) })
	t.Run("named64", func(t *testing.T) { checkNumericTables[custom64](t, 1<<60+7) })
}

func checkNumericTables[B blockgeometry.BlockID](t *testing.T, id B) {
	t.Helper()
	quad := geometry.Quad{
		Positions:   [4]geometry.Vec3{{0, 0.5, 0}, {1, 0.5, 0}, {1, 0.5, 1}, {0, 0.5, 1}},
		Normal:      geometry.Vec3{0, 1, 0},
		ShadeNormal: geometry.Vec3{0.25, 0.75, math.Copysign(0, -1)},
		UV:          [4]geometry.Vec2{{0, 0}, {1, 0}, {1, 1}, {0, 1}},
		Color:       [3]float32{0.1, 0.2, 0.3},
		Surface: material.Surface{
			Texture: "example:纹理", Mode: material.Blend, Region: [4]float32{0.1, 0.2, 0.3, 0.4},
			DoubleSided: true, Unlit: true, Opacity: 0.75,
			Frames: 32, FrameTime: 3,
		},
		CullFace: geometry.Up,
	}
	full := quad
	full.Surface.Frames = 16
	for i := range full.Positions {
		full.Positions[i][1] = 1
	}
	tables := &blockgeometry.Tables[B]{
		Index: map[B][]blockgeometry.ModelID{0: {}, id: {0}},
		Shapes: []blockgeometry.Shape{
			{Quads: []geometry.Quad{quad}, CullGroup: 7},
			{Quads: []geometry.Quad{full}, CullGroup: 7},
		},
		FullHeight: map[blockgeometry.ModelID]blockgeometry.ModelID{0: 1},
	}
	for i := range tables.Shapes {
		tables.Shapes[i].Boundary = blockgeometry.BoundaryCoverage(tables.Shapes[i].Quads)
	}
	var encoded bytes.Buffer
	if err := tables.Encode(&encoded); err != nil {
		t.Fatal(err)
	}
	loaded, err := blockgeometry.DecodeTables[B](bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	// 空边界切片统一后比较全部字段，不用近似浮点比较掩盖精度变化。
	for i := range tables.Shapes {
		for side := range tables.Shapes[i].Boundary {
			if len(tables.Shapes[i].Boundary[side]) == 0 {
				tables.Shapes[i].Boundary[side] = []geometry.BoundaryRect{}
			}
		}
	}
	if !reflect.DeepEqual(loaded, tables) {
		t.Fatal("table content changed")
	}
	if !math.Signbit(loaded.Shapes[0].Quads[0].ShadeNormal[2]) {
		t.Fatal("negative zero lost")
	}
	var again bytes.Buffer
	if err := loaded.Encode(&again); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded.Bytes(), again.Bytes()) {
		t.Fatal("encoding is not stable")
	}
	m := NewMesher(loaded)
	mesh, err := m.MeshBlock(id)
	if err != nil || len(mesh.Quads) != 1 || mesh.Quads[0] != quad {
		t.Fatalf("single block: %v", err)
	}
	mesh.Quads[0].Color[0] = -1
	if loaded.Shapes[0].Quads[0] != quad {
		t.Fatal("mesh aliases shared shape")
	}
	layer := make([]B, 4096)
	const index = (2 << 8) | (5 << 4) | 3
	layer[index], layer[index+1] = id, id
	result, err := m.MeshSubchunk(layer, Options{Meshing: meshing.Options{DisableCulling: true, DisableGreedy: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Mesh.Quads) != 2 || result.Mesh.Quads[0].Positions[0] != (geometry.Vec3{2, 4, 5}) || result.Mesh.Quads[1].Positions[0] != (geometry.Vec3{2, 4.5, 5}) {
		t.Fatal("numeric full-height switching changed")
	}
	if result.Stats.Blocks != 2 || result.Stats.Shapes != 2 {
		t.Fatalf("stats: %+v", result.Stats)
	}
	if layer[index] != id || layer[index+1] != id {
		t.Fatal("input layer changed")
	}
	if _, err := m.MeshBlock(B(1)); err == nil {
		t.Fatal("missing ID accepted")
	}
}

func TestSubchunkInputLength(t *testing.T) {
	m := NewMesher(&blockgeometry.Tables[uint32]{Index: map[uint32][]blockgeometry.ModelID{0: {}}})
	for _, count := range []int{0, 4095, 4096, 4097} {
		result, err := m.MeshSubchunk(make([]uint32, count), Options{})
		if count == 4096 {
			if err != nil || result == nil || len(result.Mesh.Quads) != 0 {
				t.Fatalf("empty subchunk: result=%+v, error=%v", result, err)
			}
		} else if err == nil || result != nil {
			t.Fatalf("length %d: result=%+v, error=%v", count, result, err)
		}
	}
}

func TestAdjacentSectionCoverageAndFluidHeight(t *testing.T) {
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	renderer := NewMesher(resources.Tables)
	solid := make([]uint32, 4096)
	for i := range solid {
		solid[i] = ir.IRBlock("minecraft:stone []").Hash()
	}
	var neighbours [6][][]uint32
	for side := range neighbours {
		neighbours[side] = [][]uint32{solid}
	}
	result, err := renderer.MeshWithNeighbours([][]uint32{solid}, neighbours, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Mesh.Quads) != 0 {
		t.Fatal("enclosed section has visible faces")
	}
	for side := range neighbours {
		exposed := neighbours
		exposed[side] = nil
		result, err = renderer.MeshWithNeighbours([][]uint32{solid}, exposed, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Mesh.Quads) != 1 {
			t.Fatalf("side %d: got %d quads", side, len(result.Mesh.Quads))
		}
	}
	lower, upper := make([]uint32, 4096), make([]uint32, 4096)
	for i := range lower {
		lower[i] = ir.IRBlock("minecraft:air []").Hash()
		upper[i] = lower[i]
	}
	water := ir.IRBlock(`minecraft:water ["level"="0"]`).Hash()
	lower[2<<8|3<<4|15] = water
	upper[2<<8|3<<4] = water
	neighbours = [6][][]uint32{}
	neighbours[3] = [][]uint32{upper}
	result, err = renderer.MeshWithNeighbours([][]uint32{lower}, neighbours, Options{Meshing: meshing.Options{DisableCulling: true, DisableGreedy: true}})
	if err != nil {
		t.Fatal(err)
	}
	top := 0.0
	for _, quad := range result.Mesh.Quads {
		for _, v := range quad.Positions {
			top = max(top, v[1])
		}
	}
	if top != 16 {
		t.Fatalf("water across section boundary has height %f", top)
	}
}

func TestFluidBottomAtSectionBoundary(t *testing.T) {
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	renderer := NewMesher(resources.Tables)
	lower, upper := make([]uint32, 4096), make([]uint32, 4096)
	for i := range lower {
		lower[i] = ir.IRBlock("minecraft:air []").Hash()
		upper[i] = lower[i]
	}
	water := ir.IRBlock(`minecraft:water ["level"="0"]`).Hash()
	lower[2<<8|3<<4|15] = water
	upper[2<<8|3<<4] = water
	var neighbours [6][][]uint32
	neighbours[2] = [][]uint32{lower}
	result, err := renderer.MeshWithNeighbours([][]uint32{upper}, neighbours, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, quad := range result.Mesh.Quads {
		if quad.Normal[1] == -1 {
			t.Fatal("internal fluid bottom remains visible")
		}
	}
}

func TestMissingShapeUsesUnregisteredReference(t *testing.T) {
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ids, err := resources.Tables.ModelIDs(ir.IRBlock("minecraft:stone []").Hash())
	if err != nil {
		t.Fatal(err)
	}
	missing := resources.Tables.Shapes[ids[0]]
	tables := &blockgeometry.Tables[uint32]{Index: map[uint32][]blockgeometry.ModelID{0: {}}}
	m := NewMesher(tables)
	center := make([]uint32, 4096)
	center[3<<8|5<<4|4] = 7
	neighbour := make([]uint32, 4096)
	neighbour[15<<8] = 8
	options := Options{MissingShape: &missing}
	result, err := m.MeshWithNeighbours([][]uint32{center, center}, [6][][]uint32{{neighbour}}, options)
	if err != nil {
		t.Fatal(err)
	}
	want := []MissingCell{
		{Pos: [3]uint8{3, 4, 5}, Layer: 0},
		{Pos: [3]uint8{3, 4, 5}, Layer: 1},
	}
	if !reflect.DeepEqual(result.Missing, want) || result.Stats.Shapes != 1 || len(tables.Index) != 1 || len(tables.Shapes) != 0 {
		t.Fatalf("missing=%v stats=%+v table=%+v", result.Missing, result.Stats, tables)
	}
	if _, err := m.MeshSubchunk(center, Options{}); err == nil {
		t.Fatal("missing center ID accepted without placeholder")
	}
	if _, err := m.MeshWithNeighbours([][]uint32{make([]uint32, 4096)}, [6][][]uint32{{neighbour}}, Options{}); err == nil {
		t.Fatal("missing neighbour ID accepted without placeholder")
	}
	tables.Index[7] = []blockgeometry.ModelID{100}
	if _, err := m.MeshSubchunk(center, options); err == nil {
		t.Fatal("placeholder hid an invalid registered model")
	}
}
