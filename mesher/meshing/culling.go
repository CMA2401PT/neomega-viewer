package meshing

import (
	"math"

	blockgeometry "neomega-blocks/geometry"
	"neomega-blocks/geometry/material"
	"neomega-viewer/mesher/geometry"
)

type inspectedFace struct {
	rect  rectangle
	valid bool
}

type inspectedShape struct {
	faces    []inspectedFace
	fullCube bool
}

// A culler belongs to one Build call. Cached inspection never outlives the
// caller's read-only input, and scratch storage is not shared between workers.
type culler struct {
	shapes map[*geometry.Shape]*inspectedShape
	clip   clipBuffers
}

func (c *culler) shape(shape *geometry.Shape) *inspectedShape {
	if cached := c.shapes[shape]; cached != nil {
		return cached
	}
	info := &inspectedShape{
		faces:    make([]inspectedFace, len(shape.Quads)),
		fullCube: unitCube(shape),
	}
	for i, quad := range shape.Quads {
		info.faces[i].rect, info.faces[i].valid = inspect(quad)
	}
	c.shapes[shape] = info
	return info
}

func (c *culler) layerScale(cell Cell, layer int) float64 {
	nearest := -1
	for _, part := range cell {
		if part.Layer < layer && part.Layer > nearest && c.shape(part.Shape).fullCube {
			nearest = part.Layer
		}
	}
	if nearest < 0 {
		return 1
	}
	return math.Pow(.99, float64(layer-nearest))
}

func (c *culler) appendCulled(
	out []geometry.Quad,
	quad geometry.Quad,
	face *inspectedFace,
	part LayerShape,
	position [3]int,
	cells *Cells,
	neighbours *Boundaries,
	stats *Stats,
) []geometry.Quad {
	if !face.valid || !face.rect.affine {
		return append(out, place(quad, position, 1))
	}
	rect := &face.rect
	c.clip.reset(rect.box)
	// Both sources of coverage operate on the same remaining rectangles. Preserve
	// traversal order so clipping, UV interpolation and output order stay identical.
	c.subtractEarlierLayers(rect, part.Layer, cells[cellIndex(position)])
	side := boundary(*rect)
	if len(c.clip.remaining) > 0 && side >= 0 && quad.CullFace == geometry.CullFace(side+1) {
		neighbor := position
		neighbor[side/2] += 2*(side%2) - 1
		if neighbor[side/2] >= 0 && neighbor[side/2] < Size {
			c.subtractNeighbor(quad, part.CullGroup, cells[cellIndex(neighbor)], side^1)
		} else if neighbours != nil {
			c.subtractNeighbor(quad, part.CullGroup, neighbours[side][BoundaryIndex(side, position)], side^1)
		}
	}
	remaining := c.clip.remaining
	if len(remaining) == 0 {
		stats.RemovedQuads++
		return out
	}
	if len(remaining) == 1 && remaining[0] == rect.box {
		return append(out, place(quad, position, 1))
	}
	clipped := len(remaining) != 1
	for _, bounds := range remaining {
		piece := rect.makeQuad(bounds)
		clipped = clipped || piece != quad
		out = append(out, place(piece, position, 1))
	}
	if clipped {
		stats.ClippedQuads++
	}
	return out
}

func (c *culler) subtractEarlierLayers(source *rectangle, layer int, cell Cell) {
	// Same-layer multipart shapes do not hide each other. Only opaque faces in
	// an earlier layer can clip a coplanar face in this layer.
	for _, part := range cell {
		if part.Layer >= layer {
			continue
		}
		faces := c.shape(part.Shape).faces
		for i := range faces {
			face := &faces[i]
			cover := &face.rect
			if !material.FullyOpaque(cover.quad.Surface) || !face.valid ||
				cover.axis != source.axis || cover.sign != source.sign || !close(cover.plane, source.plane) {
				continue
			}
			c.clip.subtract(cover.box)
			if len(c.clip.remaining) == 0 {
				return
			}
		}
	}
}

func (c *culler) subtractNeighbor(candidate geometry.Quad, group uint32, neighbor Cell, side int) {
	// Boundary rectangles already refer to the original shared shape and material.
	for _, shape := range neighbor {
		for _, cover := range shape.Boundary[side] {
			if !occludes(shape.Quads[cover.Quad], candidate, shape.CullGroup, group) {
				continue
			}
			c.clip.subtract(box{Min: [2]float64(cover.Min), Max: [2]float64(cover.Max)})
			if len(c.clip.remaining) == 0 {
				return
			}
		}
	}
}

// Ping-pong buffers keep per-face intermediates out of the heap allocation hot
// path. Complex multipart coverage can grow them; the next face reuses capacity.
type clipBuffers struct {
	storage          [2][8]box
	remaining, spare []box
}

func (c *clipBuffers) reset(source box) {
	if c.remaining == nil {
		c.remaining = c.storage[0][:0]
		c.spare = c.storage[1][:0]
	}
	c.remaining = append(c.remaining[:0], source)
}

func (c *clipBuffers) subtract(cover box) {
	after := c.spare[:0]
	for _, source := range c.remaining {
		// Handle the common unchanged/fully covered cases without allocating a
		// result slice. Partial cuts retain the shared geometry implementation.
		lo := [2]float64{max(source.Min[0], cover.Min[0]), max(source.Min[1], cover.Min[1])}
		hi := [2]float64{min(source.Max[0], cover.Max[0]), min(source.Max[1], cover.Max[1])}
		if hi[0]-lo[0] <= epsilon || hi[1]-lo[1] <= epsilon {
			after = append(after, source)
		} else if lo == source.Min && hi == source.Max {
			continue
		} else {
			after = append(after, blockgeometry.Subtract(source, cover)...)
		}
	}
	c.remaining, c.spare = after, c.remaining
}
