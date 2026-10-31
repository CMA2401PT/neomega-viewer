// Package geometry adapts shared block geometry to drawing buffers and NVMS.
package geometry

import (
	core "neomega-blocks/geometry"
	"neomega-blocks/geometry/material"
)

type Vec2 = core.Vec2
type Vec3 = core.Vec3
type Quad = core.Quad
type Bounds = core.Bounds
type BoundaryRect = core.BoundaryRect
type Shape = core.Shape
type CullFace = core.CullFace

const (
	NoCullFace = core.NoCullFace
	West       = core.West
	East       = core.East
	Down       = core.Down
	Up         = core.Up
	North      = core.North
	South      = core.South
)

// Mesh owns the scene-output methods; shared model data lives in core.
type Mesh core.Mesh

// Batch contains flat GPU attributes. UVs are texture-local, top-origin; region
// gives the atlas rectangle so the display shader can repeat within one sprite.
type Batch struct {
	Surface      material.Surface `json:"surface"`
	Positions    []float32        `json:"positions"`
	Normals      []float32        `json:"normals"`
	ShadeNormals []float32        `json:"shadeNormals"`
	UVs          []float32        `json:"uvs"`
	Colors       []float32        `json:"colors"`
	Indices      []uint32         `json:"indices"`
}

type Buffers struct {
	Batches []Batch `json:"batches"`
	Bounds  Bounds  `json:"bounds"`
	Quads   int     `json:"quads"`
}

func (mesh Mesh) Buffers() Buffers {
	result := Buffers{Batches: []Batch{}, Quads: len(mesh.Quads)}
	indices := map[material.Surface]int{}
	if len(mesh.Quads) == 0 {
		return result
	}
	result.Bounds.Min, result.Bounds.Max = mesh.Quads[0].Positions[0], mesh.Quads[0].Positions[0]
	for _, quad := range mesh.Quads {
		index, ok := indices[quad.Surface]
		if !ok {
			index = len(result.Batches)
			indices[quad.Surface] = index
			result.Batches = append(result.Batches, Batch{Surface: quad.Surface})
		}
		batch := &result.Batches[index]
		shadeNormal := quad.ShadeNormal
		if shadeNormal == (Vec3{}) {
			shadeNormal = quad.Normal
		}
		base := uint32(len(batch.Positions) / 3)
		for i, position := range quad.Positions {
			for axis := 0; axis < 3; axis++ {
				result.Bounds.Min[axis] = min(result.Bounds.Min[axis], position[axis])
				result.Bounds.Max[axis] = max(result.Bounds.Max[axis], position[axis])
				batch.Positions = append(batch.Positions, float32(position[axis]))
				batch.Normals = append(batch.Normals, float32(quad.Normal[axis]))
				batch.ShadeNormals = append(batch.ShadeNormals, float32(shadeNormal[axis]))
				batch.Colors = append(batch.Colors, quad.Color[axis])
			}
			batch.UVs = append(batch.UVs, float32(quad.UV[i][0]), float32(quad.UV[i][1]))
		}
		batch.Indices = append(batch.Indices, base, base+1, base+2, base, base+2, base+3)
	}
	return result
}
