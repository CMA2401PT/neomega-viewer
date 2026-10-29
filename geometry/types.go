// There are no neighbor queries, chunk coordinates or workers in this package.
package geometry

import "neomega-viewer/material"

type Vec3 [3]float64
type Vec2 [2]float64

type CullFace uint8

const (
	// 零值表示不可剔除，六方向编号与 Shape.Boundary 对齐。
	NoCullFace CullFace = iota
	West
	East
	Down
	Up
	North
	South
)

type Quad struct {
	Positions [4]Vec3
	Normal    Vec3
	// ShadeNormal overrides lighting only; zero uses the geometric normal.
	ShadeNormal Vec3
	UV          [4]Vec2
	Color       [3]float32
	Surface     material.Surface
	// CullFace is the rotated neighbor direction from the source model. An
	// isolated preview retains the face; a section mesher may later examine it.
	CullFace CullFace
}

type Bounds struct {
	Min Vec3 `json:"min"`
	Max Vec3 `json:"max"`
}

type Mesh struct{ Quads []Quad }

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
