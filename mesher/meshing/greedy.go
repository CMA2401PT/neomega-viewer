package meshing

import (
	"math"
	"sort"

	"neomega-blocks/geometry/material"
	"neomega-viewer/mesher/geometry"
)

type groupKey struct {
	surface material.Surface
	color   [3]float32
	axis    int
	sign    int
}

type group struct {
	reference rectangle
	bounds    []box
}

func merge(quads []geometry.Quad) []geometry.Quad {
	result := make([]geometry.Quad, 0, len(quads))
	buckets := map[groupKey][]int{}
	var groups []group
	for _, quad := range quads {
		r, ok := inspect(quad)
		if !ok || !r.affine || quad.Surface.Mode == material.Blend {
			result = append(result, quad)
			continue
		}
		key := groupKey{quad.Surface, quad.Color, r.axis, r.sign}
		found := -1
		for _, index := range buckets[key] {
			if compatible(groups[index].reference, r) {
				found = index
				break
			}
		}
		if found < 0 {
			found = len(groups)
			groups = append(groups, group{reference: r})
			buckets[key] = append(buckets[key], found)
		}
		groups[found].bounds = append(groups[found].bounds, r.box)
	}
	// Group and rectangle ordering follow input traversal, never map iteration.
	for _, group := range groups {
		if len(group.bounds) == 1 {
			result = append(result, group.reference.quad)
			continue
		}
		for _, bounds := range greedy(group.bounds) {
			result = append(result, group.reference.makeQuad(bounds))
		}
	}
	return result
}

func compatible(a, b rectangle) bool {
	if !close(a.plane, b.plane) {
		return false
	}
	for axis := 0; axis < 3; axis++ {
		if !close(a.quad.Normal[axis], b.quad.Normal[axis]) {
			return false
		}
	}
	aShade, bShade := a.quad.ShadeNormal, b.quad.ShadeNormal
	if aShade == (geometry.Vec3{}) {
		aShade = a.quad.Normal
	}
	if bShade == (geometry.Vec3{}) {
		bShade = b.quad.Normal
	}
	for axis := range aShade {
		if !close(aShade[axis], bShade[axis]) {
			return false
		}
	}
	for component := 0; component < 2; component++ {
		for axis := 0; axis < 2; axis++ {
			if !close(a.gradient[axis][component], b.gradient[axis][component]) {
				return false
			}
		}
		phase := a.offset[component] - b.offset[component]
		if !close(phase, math.Round(phase)) {
			return false
		}
	}
	return true
}

// greedy uses the actual rectangle endpoints as a 2D grid. Counts preserve
// overlapping layers; holes remain empty, including fractional model details.
func greedy(bounds []box) []box {
	var coordinates [2][]float64
	for _, bounds := range bounds {
		for axis := range coordinates {
			coordinates[axis] = append(coordinates[axis], bounds.Min[axis], bounds.Max[axis])
		}
	}
	for axis, values := range coordinates {
		sort.Float64s(values)
		unique := values[:0]
		for _, value := range values {
			if len(unique) == 0 || !close(value, unique[len(unique)-1]) {
				unique = append(unique, value)
			}
		}
		coordinates[axis] = unique
	}
	width, height := len(coordinates[0])-1, len(coordinates[1])-1
	counts := make([]int, width*height)
	coordinateIndex := func(axis int, value float64) int {
		return sort.Search(len(coordinates[axis]), func(i int) bool {
			return coordinates[axis][i] >= value-epsilon
		})
	}
	for _, bounds := range bounds {
		x0, x1 := coordinateIndex(0, bounds.Min[0]), coordinateIndex(0, bounds.Max[0])
		y0, y1 := coordinateIndex(1, bounds.Min[1]), coordinateIndex(1, bounds.Max[1])
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				counts[y*width+x]++
			}
		}
	}
	var result []box
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			for counts[y*width+x] > 0 {
				x1 := x + 1
				for x1 < width && counts[y*width+x1] > 0 {
					x1++
				}
				y1 := y + 1
				for y1 < height {
					full := true
					for xx := x; xx < x1; xx++ {
						if counts[y1*width+xx] == 0 {
							full = false
							break
						}
					}
					if !full {
						break
					}
					y1++
				}
				for yy := y; yy < y1; yy++ {
					for xx := x; xx < x1; xx++ {
						counts[yy*width+xx]--
					}
				}
				result = append(result, box{
					Min: [2]float64{coordinates[0][x], coordinates[1][y]},
					Max: [2]float64{coordinates[0][x1], coordinates[1][y1]},
				})
			}
		}
	}
	return result
}
