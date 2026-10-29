// Package meshing removes covered boundary faces and joins compatible rectangles
// in an isolated section of already baked, cell-local geometry.
package meshing

import (
	"math"

	"neomega-viewer/geometry"
	"neomega-viewer/material"
)

const Size = 16

type Cell []*geometry.Shape

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
	quads, stats := cullAndPlace(cells, options.DisableCulling)
	stats.VisibleQuads = len(quads)
	if !options.DisableGreedy {
		quads = merge(quads)
	}
	stats.OutputQuads = len(quads)
	return geometry.Mesh{Quads: quads}, stats
}

func cullAndPlace(cells *Cells, disableCulling bool) ([]geometry.Quad, Stats) {
	quads := []geometry.Quad{}
	stats := Stats{}
	for index, cell := range cells {
		position := [3]int{index >> 8, index & 15, (index >> 4) & 15}
		for _, shape := range cell {
			for _, quad := range shape.Quads {
				stats.InputQuads++
				pieces := []geometry.Quad{quad}
				if !disableCulling {
					pieces = cullQuad(quad, shape.CullGroup, position, cells)
					if len(pieces) == 0 {
						stats.RemovedQuads++
					} else if len(pieces) != 1 || pieces[0] != quad {
						stats.ClippedQuads++
					}
				}
				for _, piece := range pieces {
					quads = append(quads, place(piece, position))
				}
			}
		}
	}
	return quads, stats
}

func cullQuad(quad geometry.Quad, group uint32, position [3]int, cells *Cells) []geometry.Quad {
	unchanged := []geometry.Quad{quad}
	if quad.CullFace == geometry.NoCullFace {
		return unchanged
	}
	rect, ok := inspect(quad)
	if !ok || !rect.affine {
		return unchanged
	}
	side := boundary(rect)
	if side < 0 || quad.CullFace != geometry.CullFace(side+1) {
		return unchanged
	}
	neighbor := position
	neighbor[side/2] += 2*(side%2) - 1
	if neighbor[side/2] < 0 || neighbor[side/2] >= Size {
		return unchanged
	}
	remaining := subtractNeighbor(rect.box, quad, group, cells[cellIndex(neighbor)], side^1)
	if len(remaining) == 1 && remaining[0] == rect.box {
		return unchanged
	}
	pieces := make([]geometry.Quad, 0, len(remaining))
	for _, piece := range remaining {
		pieces = append(pieces, rect.makeQuad(piece))
	}
	return pieces
}

func subtractNeighbor(source box, candidate geometry.Quad, group uint32, neighbor Cell, side int) []box {
	// multipart 各部件的覆盖共同参与裁切，无须逐格复制或重扫邻居几何。
	remaining := []box{source}
	for _, shape := range neighbor {
		for _, cover := range shape.Boundary[side] {
			if !occludes(shape.Quads[cover.Quad], candidate, shape.CullGroup, group) {
				continue
			}
			var after []box
			for _, piece := range remaining {
				after = append(after, subtract(piece, box{lo: [2]float64(cover.Min), hi: [2]float64(cover.Max)})...)
			}
			remaining = after
			if len(remaining) == 0 {
				return nil
			}
		}
	}
	return remaining
}

func place(quad geometry.Quad, position [3]int) geometry.Quad {
	for vertex := range quad.Positions {
		for axis := range position {
			quad.Positions[vertex][axis] += float64(position[axis])
		}
	}
	return quad
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

func occludes(cover, candidate geometry.Quad, coverGroup, candidateGroup uint32) bool {
	if fullyOpaque(cover.Surface) {
		return true
	}
	// A transparent medium may remove its own internal interfaces, but never
	// mask an opaque face even if a caller accidentally reuses its group.
	return candidate.Surface.Mode != material.Opaque &&
		candidateGroup != 0 && candidateGroup == coverGroup
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
