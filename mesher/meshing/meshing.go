// Package meshing removes covered boundary faces and joins compatible rectangles
// in a section of already baked, cell-local geometry.
package meshing

import (
	blockgeometry "neomega-blocks/geometry"
	"neomega-blocks/geometry/material"
	"neomega-viewer/mesher/geometry"
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

// Boundaries holds the touching face of each adjacent section, ordered west,
// east, down, up, north, south. Each face uses BoundaryIndex for its 256 cells.
type Boundaries [6][Size * Size]Cell

func BoundaryIndex(side int, p [3]int) int {
	switch side / 2 {
	case 0:
		return p[2]*Size + p[1]
	case 1:
		return p[0]*Size + p[2]
	default:
		return p[0]*Size + p[1]
	}
}

type Options struct {
	Neighbours     *Boundaries
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
	quads, stats := cullAndPlace(cells, options.DisableCulling, options.Neighbours)
	stats.VisibleQuads = len(quads)
	if !options.DisableGreedy {
		quads = merge(quads)
	}
	stats.OutputQuads = len(quads)
	return geometry.Mesh{Quads: quads}, stats
}

func cullAndPlace(cells *Cells, disableCulling bool, neighbours *Boundaries) ([]geometry.Quad, Stats) {
	quads := []geometry.Quad{}
	stats := Stats{}
	cull := culler{shapes: make(map[*geometry.Shape]*inspectedShape)}
	for index, cell := range cells {
		position := [3]int{index >> 8, index & 15, (index >> 4) & 15}
		for _, part := range cell {
			scale := cull.layerScale(cell, part.Layer)
			var info *inspectedShape
			if !disableCulling && scale == 1 {
				info = cull.shape(part.Shape)
			}
			for i, quad := range part.Quads {
				stats.InputQuads++
				// 镂空前景下的缩进层保持原有规则，直接摆放，不参与剔面。
				if info != nil {
					quads = cull.appendCulled(
						quads,
						quad,
						&info.faces[i],
						part,
						position,
						cells,
						neighbours,
						&stats,
					)
				} else {
					quads = append(quads, place(quad, position, scale))
				}
			}
		}
	}
	return quads, stats
}

func unitCube(shape *geometry.Shape) bool {
	// 六面的矩形并集决定是否满格，镂空和透明材质同样提供缩进依据。
	var clip clipBuffers
	for _, covers := range shape.Boundary {
		clip.reset(box{Max: [2]float64{1, 1}})
		for _, cover := range covers {
			clip.subtract(box{Min: [2]float64(cover.Min), Max: [2]float64(cover.Max)})
			if len(clip.remaining) == 0 {
				break
			}
		}
		if len(clip.remaining) != 0 {
			return false
		}
	}
	return true
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

func occludes(cover, candidate geometry.Quad, coverGroup, candidateGroup uint32) bool {
	if material.FullyOpaque(cover.Surface) {
		return true
	}
	// A transparent medium may remove its own internal interfaces, but never
	// mask an opaque face even if a caller accidentally reuses its group.
	return candidate.Surface.Mode != material.Opaque &&
		candidateGroup != 0 && candidateGroup == coverGroup
}

const epsilon = blockgeometry.Epsilon

func close(a, b float64) bool { return blockgeometry.Close(a, b) }
