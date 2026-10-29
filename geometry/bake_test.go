package geometry

import (
	"image"
	"image/color"
	"math"
	"testing"

	"neomega-assets/asset"
	"neomega-viewer/blockstate"
	"neomega-viewer/material"
	"neomega-viewer/model"
	"neomega-viewer/resources"
)

func testMaterials() *material.Library {
	pixels := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			pixels.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	return material.New(asset.Atlas{Width: 16, Height: 16, Textures: map[string]asset.AtlasTexture{"stone": {SU: 1, SV: 1}}}, pixels, resources.PreviewPolicy{})
}

func close(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestCubeSlabNormalsAndDefaultUV(t *testing.T) {
	faces := map[asset.Direction]asset.Face{}
	for _, side := range directions {
		faces[asset.Direction(side)] = asset.Face{Texture: "block/stone"}
	}
	source := &model.Resolved{Elements: []asset.Element{{From: asset.Vec3{}, To: asset.Vec3{16, 8, 16}, Faces: faces}}}
	mesh, err := Bake(source, asset.Apply{}, blockstate.State{Name: "minecraft:stone"}, testMaterials())
	if err != nil {
		t.Fatal(err)
	}
	data := mesh.Buffers()
	if data.Quads != 6 || data.Bounds.Max[1] != 0.5 || len(data.Batches[0].Indices) != 36 {
		t.Fatalf("slab became a cube: %+v", data.Bounds)
	}
	for _, quad := range mesh.Quads {
		normal := unit(cross(sub(quad.Positions[1], quad.Positions[0]), sub(quad.Positions[2], quad.Positions[0])))
		if dot(normal, quad.Normal) < 0.999 {
			t.Fatal("winding and normal disagree")
		}
		if quad.Normal[0] == 1 {
			for _, uv := range quad.UV {
				if uv[1] < 0.5 || uv[1] > 1 {
					t.Fatalf("slab's default side UV ignored height: %v", uv)
				}
			}
		}
	}
}

func TestVariantRotationAndUVLock(t *testing.T) {
	source := &model.Resolved{Elements: []asset.Element{{From: asset.Vec3{}, To: asset.Vec3{16, 16, 16}, Faces: map[asset.Direction]asset.Face{
		"north": {Texture: "block/stone", CullFace: "north"}, "up": {Texture: "block/stone"},
	}}}}
	mesh, err := Bake(source, asset.Apply{Y: 90, UVLock: true}, blockstate.State{Name: "minecraft:stone"}, testMaterials())
	if err != nil {
		t.Fatal(err)
	}
	for _, quad := range mesh.Quads {
		if quad.CullFace != "" {
			if quad.CullFace != "east" || !close(quad.Normal[0], 1) {
				t.Fatalf("north did not rotate east: %+v", quad)
			}
			for _, p := range quad.Positions {
				if !close(p[0], 1) {
					t.Fatal("rotation origin is not block center")
				}
			}
		} else {
			for i, p := range quad.Positions {
				if !close(quad.UV[i][0], p[0]) || !close(quad.UV[i][1], p[2]) {
					t.Fatalf("UV lock rotated the top texture: %v %v", p, quad.UV[i])
				}
			}
		}
	}
}

func TestElementRotationProducesAPlaneNormal(t *testing.T) {
	source := &model.Resolved{Elements: []asset.Element{{From: asset.Vec3{0, 0, 8}, To: asset.Vec3{16, 16, 8}, ShadeDirectionOverride: "up", Rotation: &asset.ElementRotation{Origin: asset.Vec3{8, 8, 8}, Axis: "y", Angle: 45}, Faces: map[asset.Direction]asset.Face{"north": {Texture: "block/stone"}}}}}
	mesh, err := Bake(source, asset.Apply{}, blockstate.State{Name: "minecraft:stone"}, testMaterials())
	if err != nil || len(mesh.Quads) != 1 {
		t.Fatalf("cross plane missing: %v", err)
	}
	if mesh.Quads[0].ShadeNormal != (Vec3{0, 1, 0}) {
		t.Fatal("lighting override was lost")
	}
	n := mesh.Quads[0].Normal
	if !close(math.Abs(n[0]), math.Sqrt(0.5)) || !close(math.Abs(n[2]), math.Sqrt(0.5)) {
		t.Fatalf("normal ignored element rotation: %v", n)
	}
}

func TestCutoutVolumeFacesAndStandalonePlanesHaveDifferentSidedness(t *testing.T) {
	pixels := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	pixels.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	library := material.New(asset.Atlas{Width: 2, Height: 1, Textures: map[string]asset.AtlasTexture{"stone": {SU: 1, SV: 1}}}, pixels, resources.PreviewPolicy{})
	for _, check := range []struct {
		To     asset.Vec3
		Faces  map[asset.Direction]asset.Face
		Double bool
	}{
		{asset.Vec3{16, 16, 16}, map[asset.Direction]asset.Face{"north": {Texture: "block/stone"}}, false},
		{asset.Vec3{16, 16, 0}, map[asset.Direction]asset.Face{"north": {Texture: "block/stone"}}, true},
		{asset.Vec3{16, 16, 0}, map[asset.Direction]asset.Face{"north": {Texture: "block/stone"}, "south": {Texture: "block/stone"}}, false},
	} {
		source := &model.Resolved{Elements: []asset.Element{{To: check.To, Faces: check.Faces}}}
		mesh, err := Bake(source, asset.Apply{}, blockstate.State{Name: "minecraft:oak_leaves"}, library)
		if err != nil {
			t.Fatal(err)
		}
		for _, quad := range mesh.Quads {
			if quad.Surface.Mode != material.Cutout || quad.Surface.DoubleSided != check.Double {
				t.Fatalf("incorrect cutout sidedness: %+v", quad.Surface)
			}
		}
	}
}
