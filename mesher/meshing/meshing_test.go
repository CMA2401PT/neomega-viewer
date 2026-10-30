package meshing

import (
	"math"
	"reflect"
	"testing"

	"neomega-viewer/mesher/geometry"
	"neomega-viewer/mesher/material"
)

var opaque = material.Surface{Mode: material.Opaque, Opacity: 1}

func face(axis, sign int, plane float64, lo, hi [2]float64, surface material.Surface) geometry.Quad {
	q := geometry.Quad{Surface: surface, Color: [3]float32{1, 1, 1}}
	q.Normal[axis] = float64(sign)
	a, b := (axis+1)%3, (axis+2)%3
	corners := [4][2]float64{{lo[0], lo[1]}, {hi[0], lo[1]}, {hi[0], hi[1]}, {lo[0], hi[1]}}
	if sign < 0 {
		corners[1], corners[3] = corners[3], corners[1]
	}
	for i, corner := range corners {
		q.Positions[i][axis] = plane
		q.Positions[i][a], q.Positions[i][b] = corner[0], corner[1]
		q.UV[i] = geometry.Vec2(corner)
	}
	q.CullFace = geometry.CullFace(axis*2 + (sign+1)/2 + 1)
	return q
}

func cuboid(height float64, surface material.Surface) geometry.Mesh {
	var mesh geometry.Mesh
	hi := geometry.Vec3{1, height, 1}
	for axis := 0; axis < 3; axis++ {
		for _, sign := range []int{-1, 1} {
			plane := 0.0
			if sign > 0 {
				plane = hi[axis]
			}
			mesh.Quads = append(mesh.Quads, face(axis, sign, plane, [2]float64{}, [2]float64{hi[(axis+1)%3], hi[(axis+2)%3]}, surface))
		}
	}
	return mesh
}

func area(q geometry.Quad) float64 {
	u, v := geometry.Vec3{}, geometry.Vec3{}
	for axis := range u {
		u[axis] = q.Positions[1][axis] - q.Positions[0][axis]
		v[axis] = q.Positions[3][axis] - q.Positions[0][axis]
	}
	cross := geometry.Vec3{u[1]*v[2] - u[2]*v[1], u[2]*v[0] - u[0]*v[2], u[0]*v[1] - u[1]*v[0]}
	return math.Sqrt(cross[0]*cross[0] + cross[1]*cross[1] + cross[2]*cross[2])
}

func meshArea(mesh geometry.Mesh) float64 {
	total := 0.0
	for _, q := range mesh.Quads {
		total += area(q)
	}
	return total
}

func TestSolidSection(t *testing.T) {
	var cells fixtureCells
	cube := cuboid(1, opaque)
	original := append([]geometry.Quad(nil), cube.Quads...)
	for i := range cells {
		cells[i].Mesh = cube // deliberately share the cached slice
	}
	for _, test := range []struct {
		name    string
		options Options
		visible int
		output  int
	}{
		{"culled and merged", Options{}, 1536, 6},
		{"culled only", Options{DisableGreedy: true}, 1536, 1536},
		{"merged only", Options{DisableCulling: true}, 24576, 96},
		{"neither", Options{DisableCulling: true, DisableGreedy: true}, 24576, 24576},
	} {
		t.Run(test.name, func(t *testing.T) {
			mesh, stats := buildFixture(&cells, test.options)
			if stats.InputQuads != 24576 || stats.VisibleQuads != test.visible || stats.OutputQuads != test.output || len(mesh.Quads) != test.output {
				t.Fatalf("unexpected counts: %+v", stats)
			}
			if stats.RemovedQuads != stats.InputQuads-stats.VisibleQuads || stats.ClippedQuads != 0 {
				t.Fatalf("unexpected culling counts: %+v", stats)
			}
			if !reflect.DeepEqual(cube.Quads, original) {
				t.Fatal("Build mutated shared input")
			}
			if !test.options.DisableCulling && !close(meshArea(mesh), 6*16*16) {
				t.Fatalf("section area = %g", meshArea(mesh))
			}
			if test.output == 6 {
				for _, q := range mesh.Quads {
					for component := 0; component < 2; component++ {
						lo, hi := q.UV[0][component], q.UV[0][component]
						for _, uv := range q.UV {
							lo, hi = min(lo, uv[component]), max(hi, uv[component])
						}
						if !close(hi-lo, 16) {
							t.Fatalf("merged UVs stretch instead of repeat: %v", q.UV)
						}
					}
				}
			}
		})
	}
}

func TestPartialSlabCoverage(t *testing.T) {
	var cells fixtureCells
	cells[0].Mesh = cuboid(1, opaque)
	cells[256].Mesh = cuboid(.5, opaque)
	mesh, stats := buildFixture(&cells, Options{DisableGreedy: true})
	if stats.RemovedQuads != 1 || stats.ClippedQuads != 1 || stats.VisibleQuads != 11 || !close(meshArea(mesh), 9) {
		t.Fatalf("cube/slab: stats=%+v area=%g", stats, meshArea(mesh))
	}
	found := false
	for _, q := range mesh.Quads {
		if q.Normal == (geometry.Vec3{1, 0, 0}) && close(q.Positions[0][0], 1) {
			found = true
			if !close(area(q), .5) {
				t.Fatalf("exposed side area=%g", area(q))
			}
			for i, p := range q.Positions {
				if p[1] < .5 || !close(q.UV[i][0], p[1]) || !close(q.UV[i][1], p[2]) {
					t.Fatalf("clipping lost UV interpolation: positions=%v UV=%v", q.Positions, q.UV)
				}
			}
		}
	}
	if !found {
		t.Fatal("exposed upper half of cube disappeared")
	}
	cells[0].Mesh = cuboid(.5, opaque)
	mesh, stats = buildFixture(&cells, Options{DisableGreedy: true})
	if stats.RemovedQuads != 2 || stats.ClippedQuads != 0 || !close(meshArea(mesh), 7) {
		t.Fatalf("matching slabs: stats=%+v area=%g", stats, meshArea(mesh))
	}
}

func TestCoverageUnionAndInteriorHole(t *testing.T) {
	for _, test := range []struct {
		name    string
		covers  [][4]float64
		area    float64
		removed int
	}{
		{"union", [][4]float64{{0, 0, .4, 1}, {.4, 0, 1, 1}}, 0, 1},
		{"overlapping union", [][4]float64{{0, 0, .7, 1}, {.3, 0, 1, 1}}, 0, 1},
		{"interior hole", [][4]float64{{0, 0, .2, 1}, {.8, 0, 1, 1}, {.2, 0, .8, .2}, {.2, .8, .8, 1}}, .36, 0},
		{"central occluder", [][4]float64{{.2, .2, .8, .8}}, .64, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			var cells fixtureCells
			cells[0].Mesh.Quads = []geometry.Quad{face(0, 1, 1, [2]float64{}, [2]float64{1, 1}, opaque)}
			for _, bounds := range test.covers {
				cover := face(0, -1, 0, [2]float64{bounds[0], bounds[1]}, [2]float64{bounds[2], bounds[3]}, opaque)
				cover.CullFace = geometry.NoCullFace // actual geometry alone establishes coverage
				cells[256].Mesh.Quads = append(cells[256].Mesh.Quads, cover)
			}
			mesh, stats := buildFixture(&cells, Options{})
			exposed := 0.0
			for _, q := range mesh.Quads {
				if q.Normal[0] > 0 {
					exposed += area(q)
				}
			}
			if stats.RemovedQuads != test.removed || !close(exposed, test.area) {
				t.Fatalf("stats=%+v exposed=%g want=%g", stats, exposed, test.area)
			}
		})
	}
}

func TestTransparentInterfaces(t *testing.T) {
	glass := material.Surface{Mode: material.Blend, Opacity: .5}
	leaves := material.Surface{Mode: material.Cutout, Opacity: 1}
	for _, test := range []struct {
		name        string
		left, right material.Surface
		groups      [2]uint32
		removed     int
	}{
		{"same glass", glass, glass, [2]uint32{1, 1}, 2},
		{"different glass", glass, glass, [2]uint32{1, 2}, 0},
		{"ungrouped glass", glass, glass, [2]uint32{}, 0},
		{"glass beside stone", glass, opaque, [2]uint32{}, 1},
		{"stone beside leaves", opaque, leaves, [2]uint32{}, 1},
		{"leaves beside leaves", leaves, leaves, [2]uint32{}, 0},
		{"group never hides stone", opaque, glass, [2]uint32{1, 1}, 1},
		{"opacity prevents occlusion", material.Surface{Mode: material.Opaque, Opacity: .5}, glass, [2]uint32{}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			var cells fixtureCells
			cells[0] = fixtureCell{Mesh: cuboid(1, test.left), CullGroup: test.groups[0]}
			cells[256] = fixtureCell{Mesh: cuboid(1, test.right), CullGroup: test.groups[1]}
			_, stats := buildFixture(&cells, Options{DisableGreedy: true})
			if stats.RemovedQuads != test.removed {
				t.Fatalf("stats=%+v", stats)
			}
		})
	}
}

func TestGreedyCompatibility(t *testing.T) {
	for _, test := range []struct {
		name   string
		modify func(*geometry.Quad)
		output int
	}{
		{"same texture", func(q *geometry.Quad) {}, 1},
		{"integer UV phase", func(q *geometry.Quad) {
			for i := range q.UV {
				q.UV[i][0] += 3
			}
		}, 1},
		{"fractional UV phase", func(q *geometry.Quad) {
			for i := range q.UV {
				q.UV[i][0] += .5
			}
		}, 2},
		{"mirrored UV", func(q *geometry.Quad) {
			for i := range q.UV {
				q.UV[i][0] = 1 - q.UV[i][0]
			}
		}, 2},
		{"rotated UV", func(q *geometry.Quad) {
			for i := range q.UV {
				q.UV[i] = geometry.Vec2{q.UV[i][1], 1 - q.UV[i][0]}
			}
		}, 2},
		{"color", func(q *geometry.Quad) { q.Color[0] = .5 }, 2},
		{"surface", func(q *geometry.Quad) { q.Surface.Unlit = true }, 2},
		{"shading normal", func(q *geometry.Quad) { q.ShadeNormal = geometry.Vec3{0, 0, 1} }, 2},
		{"geometric normal", func(q *geometry.Quad) { q.Normal = geometry.Vec3{0, -1, 0} }, 2},
		{"blend", func(q *geometry.Quad) { q.Surface.Mode = material.Blend }, 2},
		{"non-affine UV", func(q *geometry.Quad) { q.UV[2][0] += .25 }, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			var cells fixtureCells
			q := face(1, 1, 1, [2]float64{}, [2]float64{1, 1}, opaque)
			cells[0].Mesh.Quads = []geometry.Quad{q}
			test.modify(&q)
			cells[256].Mesh.Quads = []geometry.Quad{q}
			mesh, _ := buildFixture(&cells, Options{})
			if len(mesh.Quads) != test.output {
				t.Fatalf("quads=%d want=%d", len(mesh.Quads), test.output)
			}
			if !close(meshArea(mesh), 2) {
				t.Fatalf("area=%g", meshArea(mesh))
			}
		})
	}
}

func TestConservativeCulling(t *testing.T) {
	for _, test := range []struct {
		name   string
		modify func(*geometry.Quad)
	}{
		{"missing cullface", func(q *geometry.Quad) { q.CullFace = geometry.NoCullFace }},
		{"wrong cullface", func(q *geometry.Quad) { q.CullFace = geometry.North }},
		{"inset", func(q *geometry.Quad) {
			for i := range q.Positions {
				q.Positions[i][0] = .75
			}
		}},
		{"tilted", func(q *geometry.Quad) { q.Positions[0][0] = .8; q.Positions[1][0] = .8 }},
		{"nonrectangular", func(q *geometry.Quad) { q.Positions[0][1] = .2 }},
		{"rotated in plane", func(q *geometry.Quad) {
			for i, p := range q.Positions {
				y, z := p[1]-.5, p[2]-.5
				q.Positions[i][1], q.Positions[i][2] = .5+(y-z)/math.Sqrt2, .5+(y+z)/math.Sqrt2
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var cells fixtureCells
			q := face(0, 1, 1, [2]float64{}, [2]float64{1, 1}, opaque)
			test.modify(&q)
			cells[0].Mesh.Quads = []geometry.Quad{q}
			cells[256].Mesh = cuboid(1, opaque)
			mesh, stats := buildFixture(&cells, Options{DisableGreedy: true})
			if mesh.Quads[0] != q || stats.ClippedQuads != 0 {
				t.Fatalf("ineligible source changed: stats=%+v", stats)
			}
		})
	}
}

func TestFractionalCoordinatesAndDeterminism(t *testing.T) {
	var cells fixtureCells
	// Adjacent rectangles inside one cell avoid any implicit block-grid rule.
	q1 := face(2, 1, .371, [2]float64{.137, .219}, [2]float64{.493, .813}, opaque)
	q2 := face(2, 1, .371, [2]float64{.493, .219}, [2]float64{.917, .813}, opaque)
	cells[0].Mesh.Quads = []geometry.Quad{q1, q2}
	mesh, stats := buildFixture(&cells, Options{})
	if len(mesh.Quads) != 1 || !close(meshArea(mesh), (.917-.137)*(.813-.219)) {
		t.Fatalf("fractional endpoints lost: %+v %+v", mesh, stats)
	}
	for i := 0; i < 5; i++ {
		again, nextStats := buildFixture(&cells, Options{})
		if !reflect.DeepEqual(mesh, again) || stats != nextStats {
			t.Fatal("nondeterministic output")
		}
	}
	// An actual narrow gap must remain a gap; no subvoxel approximation.
	for i := range q2.Positions {
		q2.Positions[i][0] += .00001
		q2.UV[i][0] += .00001
	}
	cells[0].Mesh.Quads[1] = q2
	mesh, _ = buildFixture(&cells, Options{})
	if len(mesh.Quads) != 2 {
		t.Fatal("greedy filled a fractional gap")
	}
}

func TestOverlappingLayersAndOppositeWinding(t *testing.T) {
	var cells fixtureCells
	q := face(1, 1, .5, [2]float64{}, [2]float64{1, 1}, opaque)
	back := face(1, -1, .5, [2]float64{}, [2]float64{1, 1}, opaque)
	cells[0].Mesh.Quads = []geometry.Quad{q, q, back}
	mesh, _ := buildFixture(&cells, Options{})
	if len(mesh.Quads) != 3 || !close(meshArea(mesh), 3) {
		t.Fatalf("overlapping layers collapsed: %+v", mesh)
	}
	for _, q := range mesh.Quads {
		u, v := q.Positions[1], q.Positions[2]
		p := q.Positions[0]
		winding := (u[2]-p[2])*(v[0]-p[0]) - (u[0]-p[0])*(v[2]-p[2])
		if winding*q.Normal[1] <= 0 {
			t.Fatalf("invalid outward winding: %+v", q)
		}
	}
}

func TestBlendQuadsRemainSortable(t *testing.T) {
	var cells fixtureCells
	q := face(1, 1, 1, [2]float64{}, [2]float64{1, 1}, material.Surface{Mode: material.Blend, Opacity: .5})
	cells[0].Mesh.Quads = []geometry.Quad{q}
	cells[256].Mesh.Quads = []geometry.Quad{q}
	mesh, stats := buildFixture(&cells, Options{})
	if len(mesh.Quads) != 2 || stats.VisibleQuads != stats.OutputQuads {
		t.Fatalf("blend quads merged: %+v", stats)
	}
}

func TestRotatedNeighborDoesNotMaskOpaqueFace(t *testing.T) {
	var cells fixtureCells
	q := face(0, 1, 1, [2]float64{}, [2]float64{1, 1}, opaque)
	cover := face(0, -1, 0, [2]float64{}, [2]float64{1, 1}, opaque)
	for i, p := range cover.Positions {
		y, z := p[1]-.5, p[2]-.5
		cover.Positions[i][1], cover.Positions[i][2] = .5+(y-z)/math.Sqrt2, .5+(y+z)/math.Sqrt2
	}
	cells[0].Mesh.Quads = []geometry.Quad{q}
	cells[256].Mesh.Quads = []geometry.Quad{cover}
	mesh, stats := buildFixture(&cells, Options{})
	if len(mesh.Quads) != 2 || stats.RemovedQuads != 0 || stats.ClippedQuads != 0 {
		t.Fatalf("rotated boundary geometry was treated as a solid box: %+v", stats)
	}
}

func TestOpaqueBoundary(t *testing.T) {
	left := face(0, 1, 1, [2]float64{}, [2]float64{.4, 1}, opaque)
	right := face(0, 1, 1, [2]float64{.4, 0}, [2]float64{1, 1}, opaque)
	left.CullFace, right.CullFace = geometry.NoCullFace, geometry.NoCullFace
	union := geometry.Mesh{Quads: []geometry.Quad{left, right}}
	if !OpaqueBoundary(union, "east") || OpaqueBoundary(union, "west") || OpaqueBoundary(union, "invalid") {
		t.Fatal("boundary union or requested direction incorrect")
	}
	for _, test := range []struct {
		name string
		mesh geometry.Mesh
	}{
		{"missing half", geometry.Mesh{Quads: []geometry.Quad{left}}},
		{"slab side", cuboid(.5, opaque)},
		{"cutout", cuboid(1, material.Surface{Mode: material.Cutout, Opacity: 1})},
		{"blend", cuboid(1, material.Surface{Mode: material.Blend, Opacity: .5})},
		{"reduced opacity", cuboid(1, material.Surface{Mode: material.Opaque, Opacity: .5})},
		{"hole", geometry.Mesh{Quads: []geometry.Quad{
			face(0, 1, 1, [2]float64{}, [2]float64{.2, 1}, opaque),
			face(0, 1, 1, [2]float64{.8, 0}, [2]float64{1, 1}, opaque),
			face(0, 1, 1, [2]float64{.2, 0}, [2]float64{.8, .2}, opaque),
			face(0, 1, 1, [2]float64{.2, .8}, [2]float64{.8, 1}, opaque),
		}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if OpaqueBoundary(test.mesh, "east") {
				t.Fatal("incomplete or nonopaque boundary accepted")
			}
		})
	}
	for _, side := range sideNames {
		if !OpaqueBoundary(cuboid(1, opaque), side) {
			t.Fatalf("cube missing opaque %s boundary", side)
		}
	}
	if !OpaqueBoundary(cuboid(.5, opaque), "down") || OpaqueBoundary(cuboid(.5, opaque), "up") {
		t.Fatal("slab boundary inferred from shape volume instead of face location")
	}
}

type fixtureCell struct {
	Mesh      geometry.Mesh
	CullGroup uint32
}

func TestSharedMultipartBoundaryCoverage(t *testing.T) {
	source := face(0, 1, 1, [2]float64{}, [2]float64{1, 1}, opaque)
	left := face(0, -1, 0, [2]float64{}, [2]float64{.4, 1}, opaque)
	right := face(0, -1, 0, [2]float64{.4, 0}, [2]float64{1, 1}, opaque)
	left.CullFace, right.CullFace = geometry.NoCullFace, geometry.NoCullFace
	var cells Cells
	cells[0] = Cell{{Shape: &geometry.Shape{Quads: []geometry.Quad{source}}}}
	for _, quad := range []geometry.Quad{left, right} {
		mesh := geometry.Mesh{Quads: []geometry.Quad{quad}}
		cells[256] = append(cells[256], LayerShape{Shape: &geometry.Shape{Quads: mesh.Quads, Boundary: BoundaryCoverage(mesh)}})
	}
	_, stats := Build(&cells, Options{DisableGreedy: true})
	if stats.RemovedQuads != 1 || stats.ClippedQuads != 0 {
		t.Fatalf("multipart boundary union lost: %+v", stats)
	}
	// 清空覆盖而保留面，确认运行时不会重新扫描邻居补回覆盖。
	cells[256][1].Boundary = [6][]geometry.BoundaryRect{}
	_, stats = Build(&cells, Options{DisableGreedy: true})
	if stats.RemovedQuads != 0 || stats.ClippedQuads != 1 {
		t.Fatalf("meshing ignored supplied coverage: %+v", stats)
	}
}

type fixtureCells [Size * Size * Size]fixtureCell

func TestSameCellLayerOcclusion(t *testing.T) {
	for _, test := range []struct {
		name       string
		surface    material.Surface
		frontLayer int
		frontSign  int
		width      float64
		removed    int
		clipped    int
	}{
		{"opaque", opaque, 0, 1, 1, 1, 0},
		{"partial opaque", opaque, 0, 1, .5, 0, 1},
		{"cutout", material.Surface{Mode: material.Cutout, Opacity: 1}, 0, 1, 1, 0, 0},
		{"blend", material.Surface{Mode: material.Blend, Opacity: 1}, 0, 1, 1, 0, 0},
		{"partial opacity", material.Surface{Mode: material.Opaque, Opacity: .5}, 0, 1, 1, 0, 0},
		{"same layer", opaque, 1, 1, 1, 0, 0},
		{"later layer", opaque, 2, 1, 1, 0, 0},
		{"opposite direction", opaque, 0, -1, 1, 0, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			// 内部平面与 NoCullFace 保证同格裁切不依赖格子边界或邻格许可。
			front := face(0, test.frontSign, .5, [2]float64{}, [2]float64{test.width, 1}, test.surface)
			back := face(0, 1, .5, [2]float64{}, [2]float64{1, 1}, material.Surface{Mode: material.Blend, Opacity: .7})
			front.CullFace, back.CullFace = geometry.NoCullFace, geometry.NoCullFace
			var cells Cells
			cells[0] = Cell{
				{Shape: &geometry.Shape{Quads: []geometry.Quad{front}}, Layer: test.frontLayer},
				{Shape: &geometry.Shape{Quads: []geometry.Quad{back}}, Layer: 1},
			}
			_, stats := Build(&cells, Options{DisableGreedy: true})
			if stats.RemovedQuads != test.removed || stats.ClippedQuads != test.clipped || stats.VisibleQuads != 2-test.removed {
				t.Fatalf("layer coverage: %+v", stats)
			}
		})
	}
}

func TestGreedyMergesAcrossLayers(t *testing.T) {
	quad := face(1, 1, 1, [2]float64{}, [2]float64{1, 1}, opaque)
	shape := &geometry.Shape{Quads: []geometry.Quad{quad}}
	var cells Cells
	cells[0] = Cell{{Shape: shape, Layer: 0}}
	cells[256] = Cell{{Shape: shape, Layer: 1}}
	_, stats := Build(&cells, Options{})
	if stats.InputQuads != 2 || stats.VisibleQuads != 2 || stats.OutputQuads != 1 {
		t.Fatalf("layers did not share a greedy pass: %+v", stats)
	}
}

func buildFixture(fixtures *fixtureCells, options Options) (geometry.Mesh, Stats) {
	var cells Cells
	for i, fixture := range fixtures {
		if len(fixture.Mesh.Quads) != 0 {
			cells[i] = Cell{{Shape: &geometry.Shape{
				Quads:     fixture.Mesh.Quads,
				Boundary:  BoundaryCoverage(fixture.Mesh),
				CullGroup: fixture.CullGroup,
			}}}
		}
	}
	return Build(&cells, options)
}
