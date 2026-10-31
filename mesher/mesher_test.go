package mesher

import (
	"bytes"
	"math"
	"reflect"
	"testing"

	blockgeometry "neomega-blocks/geometry"
	"neomega-blocks/geometry/material"
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
