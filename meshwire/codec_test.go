package meshwire_test

import (
	"encoding/binary"
	"reflect"
	"testing"

	"neomega-viewer/geometry"
	"neomega-viewer/material"
	"neomega-viewer/meshwire"
)

func TestRoundTripFractionalGeometryAndEmptyMesh(t *testing.T) {
	source := geometry.Mesh{Quads: []geometry.Quad{{
		Positions: [4]geometry.Vec3{{0, 0, 0}, {0, 0.5, 0}, {1, 0.5, 0}, {1, 0, 0}},
		Normal:    geometry.Vec3{0, 0, -1}, Color: [3]float32{0.25, 0.5, 1},
		UV:      [4]geometry.Vec2{{0, 0}, {0, 0.5}, {2, 0.5}, {2, 0}},
		Surface: material.Surface{Texture: "minecraft:block/stone", Region: [4]float32{0.25, 0.5, 0.125, 0.125}, Mode: material.Opaque, Opacity: 1},
	}}}
	for _, mesh := range []geometry.Buffers{source.Buffers(), (geometry.Mesh{}).Buffers()} {
		encoded, err := meshwire.Encode(mesh)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := meshwire.Decode(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(mesh, decoded) {
			t.Fatalf("round trip changed geometry: %+v", decoded)
		}
		for _, bad := range [][]byte{encoded[:10], encoded[:len(encoded)-1]} {
			if _, err := meshwire.Decode(bad); err == nil {
				t.Fatal("truncated mesh accepted")
			}
		}
		binary.LittleEndian.PutUint32(encoded[4:], 2)
		if _, err := meshwire.Decode(encoded); err == nil {
			t.Fatal("unknown version accepted")
		}
	}
}
