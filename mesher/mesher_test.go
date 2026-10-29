package mesher

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"reflect"
	"testing"

	"celler/layer_storage"
	"neomega-viewer/geometry"
	"neomega-viewer/material"
	"neomega-viewer/meshing"
)

type custom32 uint32
type custom64 uint64

func TestNumericTablesAndMeshing(t *testing.T) {
	t.Run("uint32", func(t *testing.T) { checkNumericTables[uint32](t, 0xfedcba98) })
	t.Run("uint64", func(t *testing.T) { checkNumericTables[uint64](t, 1<<60+7) })
	t.Run("named32", func(t *testing.T) { checkNumericTables[custom32](t, 0xfedcba98) })
	t.Run("named64", func(t *testing.T) { checkNumericTables[custom64](t, 1<<60+7) })
}

func checkNumericTables[B BlockID](t *testing.T, id B) {
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
		},
		CullFace: geometry.Up,
	}
	full := quad
	for i := range full.Positions {
		full.Positions[i][1] = 1
	}
	tables := &Tables[B]{
		Index: map[B][]ModelID{0: {}, id: {0}},
		Shapes: []Shape{
			{Quads: []geometry.Quad{quad}, CullGroup: 7},
			{Quads: []geometry.Quad{full}, CullGroup: 7},
		},
		FullHeight: map[ModelID]ModelID{0: 1},
	}
	for i := range tables.Shapes {
		tables.Shapes[i].Boundary = meshing.BoundaryCoverage(geometry.Mesh{Quads: tables.Shapes[i].Quads})
	}
	var encoded bytes.Buffer
	if err := tables.Encode(&encoded); err != nil {
		t.Fatal(err)
	}
	loaded, err := DecodeTables[B](bytes.NewReader(encoded.Bytes()))
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
	layer := layer_storage.MakePlainStorage(B(0))
	layer.Data[0], layer.Data[1] = id, id
	result, err := m.MeshPlainLayerStorage(layer, Options{Meshing: meshing.Options{DisableCulling: true, DisableGreedy: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Mesh.Quads) != 2 || result.Mesh.Quads[0].Positions[0][1] != 1 || result.Mesh.Quads[1].Positions[0][1] != 1.5 {
		t.Fatal("numeric full-height switching changed")
	}
	if result.Stats.Blocks != 2 || result.Stats.Shapes != 2 {
		t.Fatalf("stats: %+v", result.Stats)
	}
	if layer.Data[0] != id || layer.Data[1] != id {
		t.Fatal("input layer changed")
	}
	if _, err := m.MeshBlock(B(1)); err == nil {
		t.Fatal("missing ID accepted")
	}
}

func TestIndependentBinaryLayout(t *testing.T) {
	// 手写格式样本不经过编码器，独立约束头部和 64 位键的布局。
	fixture := []byte{
		'N', 'V', 'G', 'T', 1, 0, 8, 0,
		1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0,
		8, 7, 6, 5, 4, 3, 2, 1, 0, 0, 0, 0,
	}
	tables, err := DecodeTables[custom64](bytes.NewReader(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if ids, err := tables.ModelIDs(0x0102030405060708); err != nil || len(ids) != 0 {
		t.Fatalf("64-bit key: %v", err)
	}
	var encoded bytes.Buffer
	if err := tables.Encode(&encoded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded.Bytes(), fixture) {
		t.Fatal("layout differs from fixture")
	}
	if _, err := DecodeTables[uint32](bytes.NewReader(fixture)); err == nil {
		t.Fatal("width mismatch accepted")
	}
	for _, offset := range []int{0, 4, 6} {
		bad := bytes.Clone(fixture)
		bad[offset] = 99
		if _, err := DecodeTables[uint64](bytes.NewReader(bad)); err == nil {
			t.Fatalf("header field %d accepted", offset)
		}
	}
	for _, length := range []int{0, 31, len(fixture) - 1} {
		if _, err := DecodeTables[uint64](bytes.NewReader(fixture[:length])); err == nil {
			t.Fatalf("truncation at %d accepted", length)
		}
	}
	if binary.Size(tableHeader{}) != 32 || binary.Size(wireQuad{}) != 57 || binary.Size(wireBoundary{}) != 36 {
		t.Fatal("fixed record widths changed")
	}
	if err := tables.Encode(failingWriter{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write error lost: %v", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
