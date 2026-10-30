package meshing

import (
	"reflect"
	"testing"

	"neomega-viewer/mesher/geometry"
	"neomega-viewer/mesher/material"
)

func TestBackgroundInset(t *testing.T) {
	cutout := material.Surface{Mode: material.Cutout, Opacity: 1}
	for _, test := range []struct {
		name     string
		height   float64
		surface  material.Surface
		min, max geometry.Vec3
		faces    int
		removed  int
		clipped  int
	}{
		{"cutout cube", 1, cutout, geometry.Vec3{.0025, .0025, .0025}, geometry.Vec3{.9975, .9975, .9975}, 6, 0, 0},
		{"opaque cube", 1, opaque, geometry.Vec3{.0025, .0025, .0025}, geometry.Vec3{.9975, .9975, .9975}, 6, 0, 0},
		{"cutout slab", .5, cutout, geometry.Vec3{}, geometry.Vec3{1, 1, 1}, 6, 0, 0},
		{"opaque slab", .5, opaque, geometry.Vec3{0, .5, 0}, geometry.Vec3{1, 1, 1}, 5, 1, 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			front := insetTestShape(cuboid(test.height, test.surface))
			back := insetTestShape(cuboid(1, material.Surface{Texture: "background", Mode: material.Blend, Opacity: .7}))
			original := append([]geometry.Quad(nil), back.Quads...)
			var cells Cells
			cells[0] = Cell{{Shape: front, Layer: 0}, {Shape: back, Layer: 1}}
			mesh, stats := Build(&cells, Options{DisableGreedy: true})
			assertBackgroundBounds(t, mesh, test.min, test.max, test.faces)
			if stats.InputQuads != 12 || stats.RemovedQuads != test.removed || stats.ClippedQuads != test.clipped {
				t.Fatalf("inset changed culling stats: %+v", stats)
			}
			if !reflect.DeepEqual(back.Quads, original) {
				t.Fatal("inset mutated shared geometry")
			}
		})
	}
}

func TestInsetKeepsFacesAndOriginalNeighborCoverage(t *testing.T) {
	front := insetTestShape(cuboid(1, material.Surface{Mode: material.Cutout, Opacity: 1}))
	back := insetTestShape(cuboid(1, material.Surface{Texture: "background", Mode: material.Blend, Opacity: .7}))
	back.CullGroup = 7
	var cells Cells
	cells[0] = Cell{{Shape: front, Layer: 0}, {Shape: back, Layer: 1}}
	cells[256] = Cell{{Shape: back, Layer: 0}}
	mesh, stats := Build(&cells, Options{DisableGreedy: true})
	// 缩进层保留朝向邻格的面，邻格仍按原始覆盖删除朝向缩进层的面。
	if stats.InputQuads != 18 || stats.RemovedQuads != 1 || stats.ClippedQuads != 0 || stats.OutputQuads != 17 {
		t.Fatalf("inset/neighbor culling: %+v", stats)
	}
	insetFaces, neighborFaces := 0, 0
	for _, quad := range mesh.Quads {
		if quad.Surface.Texture != "background" {
			continue
		}
		if quad.Positions[0][0] < 1 {
			insetFaces++
		} else {
			neighborFaces++
			if quad.Normal[0] == -1 {
				t.Fatal("neighbor ignored original background boundary")
			}
		}
	}
	if insetFaces != 6 || neighborFaces != 5 {
		t.Fatalf("inset faces=%d, neighbor faces=%d", insetFaces, neighborFaces)
	}
}

func TestInsetUsesNearestFullCubeLayer(t *testing.T) {
	front := insetTestShape(cuboid(1, material.Surface{Mode: material.Cutout, Opacity: 1}))
	// 每个方向用重叠矩形拼满，避免把满立方体误写成单张整面的判断。
	for side := range front.Boundary {
		front.Boundary[side] = []geometry.BoundaryRect{
			{Quad: side, Min: geometry.Vec2{}, Max: geometry.Vec2{.6, 1}},
			{Quad: side, Min: geometry.Vec2{.4, 0}, Max: geometry.Vec2{1, 1}},
		}
	}
	back := insetTestShape(cuboid(1, material.Surface{Texture: "background", Mode: material.Blend, Opacity: .7}))
	for _, middleHeight := range []float64{.5, 1} {
		middle := insetTestShape(cuboid(middleHeight, material.Surface{Mode: material.Cutout, Opacity: 1}))
		var cells Cells
		cells[0] = Cell{{Shape: back, Layer: 3}, {Shape: front, Layer: 0}, {Shape: middle, Layer: 2}}
		mesh, _ := Build(&cells, Options{DisableGreedy: true})
		lo, hi := .0074625625, .9925374375
		if middleHeight == 1 {
			lo, hi = .0025, .9975
		}
		assertBackgroundBounds(t, mesh, geometry.Vec3{lo, lo, lo}, geometry.Vec3{hi, hi, hi}, 6)
	}
}

func insetTestShape(mesh geometry.Mesh) *geometry.Shape {
	return &geometry.Shape{Quads: mesh.Quads, Boundary: BoundaryCoverage(mesh)}
}

func assertBackgroundBounds(t *testing.T, mesh geometry.Mesh, wantMin, wantMax geometry.Vec3, wantFaces int) {
	t.Helper()
	lo, hi := geometry.Vec3{1, 1, 1}, geometry.Vec3{}
	faces := 0
	for _, quad := range mesh.Quads {
		if quad.Surface.Texture != "background" {
			continue
		}
		faces++
		for _, p := range quad.Positions {
			for axis := range p {
				lo[axis], hi[axis] = min(lo[axis], p[axis]), max(hi[axis], p[axis])
			}
		}
	}
	if faces != wantFaces {
		t.Fatalf("background faces=%d, want %d", faces, wantFaces)
	}
	for axis := range lo {
		if !close(lo[axis], wantMin[axis]) || !close(hi[axis], wantMax[axis]) {
			t.Fatalf("background bounds=%v..%v, want %v..%v", lo, hi, wantMin, wantMax)
		}
	}
}
