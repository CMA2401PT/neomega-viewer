package meshing

import (
	"math"

	"neomega-viewer/geometry"
)

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
	r := rectangle{quad: quad, axis: -1}
	lo, hi := quad.Positions[0], quad.Positions[0]
	for _, p := range quad.Positions {
		for axis := range p {
			if math.IsNaN(p[axis]) || math.IsInf(p[axis], 0) {
				return r, false
			}
			lo[axis], hi[axis] = min(lo[axis], p[axis]), max(hi[axis], p[axis])
		}
	}
	for axis := range lo {
		if close(lo[axis], hi[axis]) {
			if r.axis >= 0 {
				return r, false
			}
			r.axis = axis
		}
	}
	if r.axis < 0 {
		return r, false
	}
	// Cyclic axes ensure the canonical (a,b) orientation has positive normal.
	r.a, r.b = (r.axis+1)%3, (r.axis+2)%3
	r.plane = quad.Positions[0][r.axis]
	r.box = box{[2]float64{lo[r.a], lo[r.b]}, [2]float64{hi[r.a], hi[r.b]}}
	var seen [2][2]bool
	var uv [2][2]geometry.Vec2
	for i, p := range quad.Positions {
		for component, axis := range [2]int{r.a, r.b} {
			switch {
			case close(p[axis], lo[axis]):
				r.corner[i][component] = 0
			case close(p[axis], hi[axis]):
				r.corner[i][component] = 1
			default:
				return r, false
			}
		}
		corner := r.corner[i]
		if seen[corner[0]][corner[1]] {
			return r, false
		}
		seen[corner[0]][corner[1]] = true
		uv[corner[0]][corner[1]] = quad.UV[i]
	}
	for i, corner := range r.corner {
		next := r.corner[(i+1)%4]
		if (corner[0] == next[0]) == (corner[1] == next[1]) {
			return r, false // diagonal edges / bow-tie ordering
		}
	}
	p, q, s := r.corner[0], r.corner[1], r.corner[2]
	r.sign = (q[0]-p[0])*(s[1]-p[1]) - (q[1]-p[1])*(s[0]-p[0])
	r.affine = true
	for component := 0; component < 2; component++ {
		r.gradient[0][component] = (uv[1][0][component] - uv[0][0][component]) / (r.hi[0] - r.lo[0])
		r.gradient[1][component] = (uv[0][1][component] - uv[0][0][component]) / (r.hi[1] - r.lo[1])
		r.offset[component] = uv[0][0][component] - r.gradient[0][component]*r.lo[0] - r.gradient[1][component]*r.lo[1]
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
		p[r.a], p[r.b] = bounds.lo[0], bounds.lo[1]
		if corner[0] == 1 {
			p[r.a] = bounds.hi[0]
		}
		if corner[1] == 1 {
			p[r.b] = bounds.hi[1]
		}
		quad.Positions[i] = p
		for component := 0; component < 2; component++ {
			quad.UV[i][component] = r.gradient[0][component]*p[r.a] + r.gradient[1][component]*p[r.b] + r.offset[component]
		}
	}
	return quad
}
