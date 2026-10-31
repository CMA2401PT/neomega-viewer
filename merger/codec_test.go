package merger

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"

	"neomega-blocks/geometry/material"
	"neomega-viewer/blockactor"
	"neomega-viewer/effector"
	"neomega-viewer/mesher/geometry"
)

func TestRoundTripAndAlignment(t *testing.T) {
	mesh := geometry.Mesh{Quads: []geometry.Quad{{
		Positions:   [4]geometry.Vec3{{1, 2, 3}, {1, 3, 3}, {2, 3, 3}, {2, 2, 3}},
		Normal:      geometry.Vec3{0, 0, -1},
		ShadeNormal: geometry.Vec3{0, 0, -1},
		UV:          [4]geometry.Vec2{{.0625, 0}, {.0625, 1}, {.9375, 1}, {.9375, 0}},
		Color:       [3]float32{1, 1, 1},
		Surface:     material.Surface{Texture: "test", Mode: material.Opaque, Opacity: 1, Frames: 1},
	}}}
	for _, frame := range []Frame{
		{
			Mesh: mesh,
			Effects: []effector.Message{
				{Pos: [3]uint8{1, 2, 3}, Effect: "enchanting_table_book"},
			},
			Actors: []blockactor.Actor{
				{Pos: [3]uint8{4, 5, 6}, Mesh: mesh},
				{Pos: [3]uint8{7, 8, 9}, Mesh: mesh},
			},
		},
		{Mesh: geometry.Mesh{Quads: []geometry.Quad{}}, Effects: []effector.Message{}, Actors: []blockactor.Actor{}},
	} {
		var encoded bytes.Buffer
		if err := frame.Marshal(&encoded); err != nil {
			t.Fatal(err)
		}
		data := encoded.Bytes()
		var head header
		var directory [3]section
		r := bytes.NewReader(data)
		if err := binary.Read(r, binary.LittleEndian, &head); err != nil {
			t.Fatal(err)
		}
		if err := binary.Read(r, binary.LittleEndian, &directory); err != nil {
			t.Fatal(err)
		}
		if string(head.Magic[:]) != "NVFR" || head.Version != Version || head.Sections != 3 {
			t.Fatalf("header=%+v", head)
		}
		for i, entry := range directory {
			if entry.Type != uint32(i+1) || entry.Offset%4 != 0 {
				t.Fatalf("unaligned directory entry: %+v", entry)
			}
			end := entry.Offset + entry.Length
			if i+1 < len(directory) && directory[i+1].Offset != align(end) {
				t.Fatal("directory offsets do not describe padded payloads")
			}
			for _, padding := range data[end:align(end)] {
				if padding != 0 {
					t.Fatal("nonzero alignment padding")
				}
			}
		}
		// 嵌入的 NVMS 必须可独立读取，不能借用后面段的字节。
		entry := directory[0]
		decodedMesh, err := geometry.UnmarshalMesh(bytes.NewReader(data[entry.Offset : entry.Offset+entry.Length]))
		if err != nil || !reflect.DeepEqual(frame.Mesh, decodedMesh) {
			t.Fatalf("static payload changed: %v", err)
		}
		decoded, err := Unmarshal(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(frame, decoded) {
			t.Fatalf("round trip differs:\nwant %+v\ngot %+v", frame, decoded)
		}
		var again bytes.Buffer
		if err := decoded.Marshal(&again); err != nil || !bytes.Equal(data, again.Bytes()) {
			t.Fatalf("wire round trip differs: %v", err)
		}
		t.Logf("bytes=%d sections=%+v", len(data), directory)
	}
}
