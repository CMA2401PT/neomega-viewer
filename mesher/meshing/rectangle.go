package meshing

import (
	blockgeometry "neomega-blocks/geometry"
	"neomega-viewer/mesher/geometry"
)

type box = blockgeometry.Rect

type rectangle struct {
	quad geometry.Quad
	box
	axis, a, b int
	plane      float64
	sign       int
	// corner identifies each input vertex as a low/high pair, retaining winding.
	corner [4][2]int
	// uv = gradient[0]*a + gradient[1]*b + offset, per UV component.
	gradient [2]geometry.Vec2
	offset   geometry.Vec2
	affine   bool
}

// inspect accepts only planar, axis-aligned rectangles with cyclic corners.
// UV affinity is separate: a geometrically sound face can still occlude.
func inspect(quad geometry.Quad) (rectangle, bool) {
	shape, ok := blockgeometry.InspectRectangle(quad.Positions)
	if !ok {
		return rectangle{}, false
	}
	r := rectangle{
		quad: quad, box: shape.Rect,
		axis: shape.Axis, a: shape.A, b: shape.B,
		plane: shape.Plane, sign: shape.Sign, corner: shape.Corners,
	}
	var uv [2][2]geometry.Vec2
	for i, corner := range r.corner {
		uv[corner[0]][corner[1]] = quad.UV[i]
	}
	r.affine = true
	for component := 0; component < 2; component++ {
		r.gradient[0][component] = (uv[1][0][component] - uv[0][0][component]) / (r.Max[0] - r.Min[0])
		r.gradient[1][component] = (uv[0][1][component] - uv[0][0][component]) / (r.Max[1] - r.Min[1])
		r.offset[component] = uv[0][0][component] - r.gradient[0][component]*r.Min[0] - r.gradient[1][component]*r.Min[1]
		if !close(uv[1][1][component], uv[1][0][component]+uv[0][1][component]-uv[0][0][component]) {
			r.affine = false
		}
	}
	return r, true
}

func boundary(r rectangle) int {
	if r.sign < 0 && close(r.plane, 0) {
		return r.axis * 2
	}
	if r.sign > 0 && close(r.plane, 1) {
		return r.axis*2 + 1
	}
	return -1
}

func (r rectangle) makeQuad(bounds box) geometry.Quad {
	quad := r.quad
	for i, corner := range r.corner {
		p := geometry.Vec3{}
		p[r.axis] = r.plane
		p[r.a], p[r.b] = bounds.Min[0], bounds.Min[1]
		if corner[0] == 1 {
			p[r.a] = bounds.Max[0]
		}
		if corner[1] == 1 {
			p[r.b] = bounds.Max[1]
		}
		quad.Positions[i] = p
		for component := 0; component < 2; component++ {
			quad.UV[i][component] = r.gradient[0][component]*p[r.a] + r.gradient[1][component]*p[r.b] + r.offset[component]
		}
	}
	return quad
}
