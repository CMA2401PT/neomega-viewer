// Package meshing removes covered boundary faces and joins compatible rectangles
// in an isolated section of already baked, cell-local geometry.
package meshing

import (
	"math"

	"neomega-viewer/mesher/geometry"
	"neomega-viewer/mesher/material"
)

const Size = 16

type LayerShape struct {
	// 层号属于摆放位置，嵌入引用可保持共享几何只读。
	*geometry.Shape
	Layer int
}

type Cell []LayerShape

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
		for _, part := range cell {
			scale := layerScale(cell, part.Layer)
			for _, quad := range part.Quads {
				stats.InputQuads++
				pieces := []geometry.Quad{quad}
				// 前景镂空使共面无法裁切时，用微小缩进避开深度冲突，缩进层的边界交给前景。
				if !disableCulling && scale == 1 {
					pieces = cullQuad(quad, part, position, cells)
					if len(pieces) == 0 {
						stats.RemovedQuads++
					} else if len(pieces) != 1 || pieces[0] != quad {
						stats.ClippedQuads++
					}
				}
				for _, piece := range pieces {
					quads = append(quads, place(piece, position, scale))
				}
			}
		}
	}
	return quads, stats
}

func layerScale(cell Cell, layer int) float64 {
	nearest := -1
	for _, part := range cell {
		if part.Layer < layer && part.Layer > nearest && unitCube(part.Shape) {
			nearest = part.Layer
		}
	}
	if nearest < 0 {
		return 1
	}
	return math.Pow(.995, float64(layer-nearest))
}

func unitCube(shape *geometry.Shape) bool {
	// 六面的矩形并集决定是否满格，镂空和透明材质同样提供缩进依据。
	for _, covers := range shape.Boundary {
		remaining := []box{{hi: [2]float64{1, 1}}}
		for _, cover := range covers {
			var after []box
			for _, piece := range remaining {
				after = append(after, subtract(piece, box{lo: [2]float64(cover.Min), hi: [2]float64(cover.Max)})...)
			}
			remaining = after
		}
		if len(remaining) != 0 {
			return false
		}
	}
	return true
}

func cullQuad(quad geometry.Quad, part LayerShape, position [3]int, cells *Cells) []geometry.Quad {
	unchanged := []geometry.Quad{quad}
	rect, ok := inspect(quad)
	if !ok || !rect.affine {
		return unchanged
	}
	// 两种覆盖都作用于原始面的剩余矩形，最终只统计一次删除或裁切。
	remaining := subtractEarlierLayers(rect, part.Layer, cells[cellIndex(position)])
	side := boundary(rect)
	if side >= 0 && quad.CullFace == geometry.CullFace(side+1) {
		neighbor := position
		neighbor[side/2] += 2*(side%2) - 1
		if neighbor[side/2] >= 0 && neighbor[side/2] < Size {
			remaining = subtractNeighbor(remaining, quad, part.CullGroup, cells[cellIndex(neighbor)], side^1)
		}
	}
	if len(remaining) == 1 && remaining[0] == rect.box {
		return unchanged
	}
	pieces := make([]geometry.Quad, 0, len(remaining))
	for _, piece := range remaining {
		pieces = append(pieces, rect.makeQuad(piece))
	}
	return pieces
}

func subtractEarlierLayers(source rectangle, layer int, cell Cell) []box {
	// 同层 multipart 没有先后关系；透明与镂空面不能作为实体覆盖。
	remaining := []box{source.box}
	for _, part := range cell {
		if part.Layer >= layer {
			continue
		}
		for _, quad := range part.Quads {
			if !fullyOpaque(quad.Surface) {
				continue
			}
			cover, ok := inspect(quad)
			if !ok || cover.axis != source.axis || cover.sign != source.sign || !close(cover.plane, source.plane) {
				continue
			}
			var after []box
			for _, piece := range remaining {
				after = append(after, subtract(piece, cover.box)...)
			}
			remaining = after
			if len(remaining) == 0 {
				return nil
			}
		}
	}
	return remaining
}

func subtractNeighbor(remaining []box, candidate geometry.Quad, group uint32, neighbor Cell, side int) []box {
	// multipart 各部件的覆盖共同参与裁切，无须逐格复制或重扫邻居几何。
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

func place(quad geometry.Quad, position [3]int, scale float64) geometry.Quad {
	for vertex := range quad.Positions {
		for axis := range position {
			if scale != 1 {
				quad.Positions[vertex][axis] = .5 + (quad.Positions[vertex][axis]-.5)*scale
			}
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
