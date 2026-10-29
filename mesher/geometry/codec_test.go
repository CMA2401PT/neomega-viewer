package geometry_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"reflect"
	"testing"

	"neomega-viewer/mesher/geometry"
	"neomega-viewer/mesher/material"
)

func sampleQuad() geometry.Quad {
	return geometry.Quad{
		Positions: [4]geometry.Vec3{{0, 0, 0}, {0, 0.5, 0}, {1, 0.5, 0}, {1, 0, 0}},
		Normal:    geometry.Vec3{0, 0, -1}, Color: [3]float32{0.25, 0.5, 1},
		UV:      [4]geometry.Vec2{{0, 0}, {0, 0.5}, {2, 0.5}, {2, 0}},
		Surface: material.Surface{Texture: "minecraft:block/stone", Region: [4]float32{0.25, 0.5, 0.125, 0.125}, Mode: material.Opaque, Opacity: 1, Frames: 1},
	}
}

func sampleMesh() geometry.Mesh {
	first := sampleQuad()
	second := first
	second.Surface.Texture = "example:纹理"
	second.Surface.Mode = material.Blend
	second.Surface.DoubleSided = true
	second.Surface.Opacity = 0.5
	second.Surface.Frames = 32
	second.Surface.FrameTime = 3
	second.ShadeNormal = geometry.Vec3{0, 1, 0}
	for i := range second.Positions {
		second.Positions[i][2] = 1.0 / 3
	}
	third := first
	for i := range third.Positions {
		third.Positions[i][0] += 2
	}
	// 交错材质可检查分批后的属性和索引顺序。
	return geometry.Mesh{Quads: []geometry.Quad{first, second, third}}
}

func TestRoundTripFractionalGeometryAndEmptyMesh(t *testing.T) {
	for _, mesh := range []geometry.Mesh{sampleMesh(), {}} {
		var encoded bytes.Buffer
		if err := mesh.Marshal(&encoded); err != nil {
			t.Fatal(err)
		}
		decoded, err := geometry.UnmarshalMesh(bytes.NewReader(encoded.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		want := mesh.Buffers()
		// NVMS 坐标本来就是 float32，比较绘制精度而不是烘焙精度。
		for axis := range want.Bounds.Min {
			want.Bounds.Min[axis] = float64(float32(want.Bounds.Min[axis]))
			want.Bounds.Max[axis] = float64(float32(want.Bounds.Max[axis]))
		}
		if !reflect.DeepEqual(want, decoded.Buffers()) {
			t.Fatalf("round trip changed geometry: %+v", decoded)
		}
		var again bytes.Buffer
		if err := decoded.Marshal(&again); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(encoded.Bytes(), again.Bytes()) {
			t.Fatal("round trip changed NVMS bytes")
		}
		for _, bad := range [][]byte{encoded.Bytes()[:10], encoded.Bytes()[:encoded.Len()-1]} {
			if _, err := geometry.UnmarshalMesh(bytes.NewReader(bad)); err == nil {
				t.Fatal("truncated mesh accepted")
			}
		}
		binary.LittleEndian.PutUint32(encoded.Bytes()[4:], 2)
		if _, err := geometry.UnmarshalMesh(bytes.NewReader(encoded.Bytes())); err == nil {
			t.Fatal("unknown version accepted")
		}
	}
}

func TestMarshalMatchesMeshwireV1(t *testing.T) {
	// 样本保留旧 meshwire 的几何字节，材质 JSON 补入图集帧信息。
	want, err := os.ReadFile("testdata/meshwire-v1.nvmesh")
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	if err := sampleMesh().Marshal(&got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("Marshal changed the NVMS v1 layout")
	}
}

func TestQuadRoundTrip(t *testing.T) {
	quad := sampleQuad()
	quad.CullFace = geometry.North
	var encoded bytes.Buffer
	if err := quad.Marshal(&encoded); err != nil {
		t.Fatal(err)
	}
	decoded, err := geometry.UnmarshalQuad(&encoded)
	if err != nil {
		t.Fatal(err)
	}
	quad.CullFace = geometry.NoCullFace
	quad.ShadeNormal = quad.Normal
	if decoded != quad {
		t.Fatalf("round trip changed quad: %+v", decoded)
	}
}

type failedIO struct{ err error }

func (f failedIO) Read([]byte) (int, error)  { return 0, f.err }
func (f failedIO) Write([]byte) (int, error) { return 0, f.err }

func TestCodecPropagatesIOErrors(t *testing.T) {
	failure := failedIO{err: io.ErrClosedPipe}
	if err := sampleMesh().Marshal(failure); !errors.Is(err, failure.err) {
		t.Fatalf("writer error: %v", err)
	}
	if _, err := geometry.UnmarshalMesh(failure); !errors.Is(err, failure.err) {
		t.Fatalf("reader error: %v", err)
	}
}
