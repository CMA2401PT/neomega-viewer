// Package meshing removes covered boundary faces and joins compatible rectangles
// in an isolated section of already baked, cell-local geometry.
package meshing

import (
	"math"

	"neomega-viewer/geometry"
	"neomega-viewer/material"
)

const Size = 16

type Cell struct {
	Mesh      geometry.Mesh
	CullGroup string
}

// Cells uses (x<<8)|(z<<4)|y indexing. Mesh coordinates remain local to a cell.
type Cells [Size * Size * Size]Cell

type Options struct {
	DisableCulling bool
	DisableGreedy  bool
}

type Stats struct {
	InputQuads   int `json:"inputQuads"`
	RemovedQuads int `json:"removedQuads"`
	ClippedQuads int `json:"clippedQuads"`
	VisibleQuads int `json:"visibleQuads"`
	OutputQuads  int `json:"outputQuads"`
}

// Build never mutates cells or their shared mesh slices. RemovedQuads counts
// fully hidden source quads; ClippedQuads counts partially hidden source quads.
// VisibleQuads counts the remaining pieces before optional greedy merging.
func Build(cells *Cells, options Options) (geometry.Mesh, Stats) {
	result := geometry.Mesh{Quads: []geometry.Quad{}}
	stats := Stats{}
	// Cache actual boundary geometry, not block-wide solidity. Neighbor faces
	// need no cullface declaration to cover another cell's cullable face.
	var boundaries [len(Cells{})][6][]rectangle
	if !options.DisableCulling {
		for index := range cells {
			for _, quad := range cells[index].Mesh.Quads {
				if rect, ok := inspect(quad); ok {
					if side := boundary(rect); side >= 0 {
						boundaries[index][side] = append(boundaries[index][side], rect)
					}
				}
			}
		}
	}
	for index := range cells {
		cell := &cells[index]
		position := [3]int{index >> 8, index & 15, (index >> 4) & 15}
		for _, quad := range cell.Mesh.Quads {
			stats.InputQuads++
			pieces := []geometry.Quad{quad}
			if !options.DisableCulling {
				if rect, ok := inspect(quad); ok && rect.affine {
					side := boundary(rect)
					if side >= 0 && quad.CullFace == sideNames[side] {
						neighbor := position
						neighbor[side/2] += 2*(side%2) - 1
						if neighbor[side/2] >= 0 && neighbor[side/2] < Size {
							next := &cells[cellIndex(neighbor)]
							remaining := []box{rect.box}
							for _, cover := range boundaries[cellIndex(neighbor)][side^1] {
								if !occludes(cover.quad, quad, next.CullGroup, cell.CullGroup) {
									continue
								}
								var after []box
								for _, piece := range remaining {
									after = append(after, subtract(piece, cover.box)...)
								}
								remaining = after
							}
							if len(remaining) == 0 {
								stats.RemovedQuads++
								pieces = nil
							} else if len(remaining) != 1 || remaining[0] != rect.box {
								stats.ClippedQuads++
								pieces = nil
								for _, piece := range remaining {
									pieces = append(pieces, rect.makeQuad(piece))
								}
							}
						}
					}
				}
			}
			for _, piece := range pieces {
				for vertex := range piece.Positions {
					for axis := range position {
						piece.Positions[vertex][axis] += float64(position[axis])
					}
				}
				result.Quads = append(result.Quads, piece)
			}
		}
	}
	stats.VisibleQuads = len(result.Quads)
	if !options.DisableGreedy {
		result.Quads = merge(result.Quads)
	}
	stats.OutputQuads = len(result.Quads)
	return result, stats
}

var sideNames = [6]string{"west", "east", "down", "up", "north", "south"}

func cellIndex(p [3]int) int { return (p[0] << 8) | (p[2] << 4) | p[1] }

// OpaqueBoundary reports whether actual opaque rectangles cover the entire
// requested local unit face. It can establish a solid attachment face without
// assuming the rest of the model is a full cube. Unknown side names are false.
func OpaqueBoundary(mesh geometry.Mesh, side string) bool {
	sideIndex := -1
	for i, name := range sideNames {
		if name == side {
			sideIndex = i
			break
		}
	}
	if sideIndex < 0 {
		return false
	}
	remaining := []box{{hi: [2]float64{1, 1}}}
	for _, quad := range mesh.Quads {
		if !fullyOpaque(quad.Surface) {
			continue
		}
		r, ok := inspect(quad)
		if !ok || boundary(r) != sideIndex {
			continue
		}
		var after []box
		for _, piece := range remaining {
			after = append(after, subtract(piece, r.box)...)
		}
		remaining = after
		if len(remaining) == 0 {
			return true
		}
	}
	return false
}

func fullyOpaque(surface material.Surface) bool {
	return surface.Mode == material.Opaque && float64(surface.Opacity) >= 1-epsilon
}

func occludes(cover, candidate geometry.Quad, coverGroup, candidateGroup string) bool {
	if fullyOpaque(cover.Surface) {
		return true
	}
	// A transparent medium may remove its own internal interfaces, but never
	// mask an opaque face even if a caller accidentally reuses its group.
	return candidate.Surface.Mode != material.Opaque &&
		candidateGroup != "" && candidateGroup == coverGroup
}

// A small tolerance accepts floating-point noise from right-angle rotations.
// Coordinates are never rounded onto a voxel/subvoxel lattice.
const epsilon = 1e-9

func close(a, b float64) bool { return math.Abs(a-b) <= epsilon }

type box struct{ lo, hi [2]float64 }

// subtract produces disjoint rectangles covering exactly source minus cover.
func subtract(source, cover box) []box {
	lo := [2]float64{max(source.lo[0], cover.lo[0]), max(source.lo[1], cover.lo[1])}
	hi := [2]float64{min(source.hi[0], cover.hi[0]), min(source.hi[1], cover.hi[1])}
	if hi[0]-lo[0] <= epsilon || hi[1]-lo[1] <= epsilon {
		return []box{source}
	}
	pieces := []box{
		{source.lo, [2]float64{lo[0], source.hi[1]}},
		{[2]float64{hi[0], source.lo[1]}, source.hi},
		{[2]float64{lo[0], source.lo[1]}, [2]float64{hi[0], lo[1]}},
		{[2]float64{lo[0], hi[1]}, [2]float64{hi[0], source.hi[1]}},
	}
	result := make([]box, 0, 4)
	for _, piece := range pieces {
		if piece.hi[0]-piece.lo[0] > epsilon && piece.hi[1]-piece.lo[1] > epsilon {
			result = append(result, piece)
		}
	}
	return result
}
