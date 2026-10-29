package geometry

import (
	"fmt"
	"math"

	"neomega-assets/asset"
	"neomega-viewer/blockstate"
	"neomega-viewer/material"
	"neomega-viewer/model"
)

func blockRotate(v Vec3, application asset.Apply) Vec3 {
	v = rotate(v, "x", -application.X)
	v = rotate(v, "y", -application.Y)
	return rotate(v, "z", -application.Z)
}

func elementRotate(vertex Vec3, rotation *asset.ElementRotation) Vec3 {
	if rotation == nil {
		return vertex
	}
	origin := Vec3(rotation.Origin)
	v := sub(vertex, origin)
	if rotation.Axis != "" {
		v = rotate(v, rotation.Axis, rotation.Angle)
		if rotation.Rescale {
			scale := 1 / math.Cos(rotation.Angle*math.Pi/180)
			for i, axis := range []string{"x", "y", "z"} {
				if axis != rotation.Axis {
					v[i] *= scale
				}
			}
		}
	} else {
		v = rotate(v, "x", rotation.X)
		v = rotate(v, "y", rotation.Y)
		v = rotate(v, "z", rotation.Z)
	}
	return add(v, origin)
}

func defaultUV(side string, lo, hi Vec3) asset.UV {
	switch side {
	case "down":
		return asset.UV{lo[0], 16 - hi[2], hi[0], 16 - lo[2]}
	case "up":
		return asset.UV{lo[0], lo[2], hi[0], hi[2]}
	case "north":
		return asset.UV{16 - hi[0], 16 - hi[1], 16 - lo[0], 16 - lo[1]}
	case "south":
		return asset.UV{lo[0], 16 - hi[1], hi[0], 16 - lo[1]}
	case "west":
		return asset.UV{lo[2], 16 - hi[1], hi[2], 16 - lo[1]}
	default:
		return asset.UV{16 - hi[2], 16 - hi[1], 16 - lo[2], 16 - lo[1]}
	}
}

func lockedUV(local Vec2, side string, application asset.Apply) Vec2 {
	before := corners(side, Vec3{}, Vec3{1, 1, 1})
	after := corners(direction(blockRotate(normals[side], application)), Vec3{}, Vec3{1, 1, 1})
	sourceU := blockRotate(sub(before[3], before[0]), application)
	sourceV := blockRotate(sub(before[1], before[0]), application)
	worldU, worldV := sub(after[3], after[0]), sub(after[1], after[0])
	delta := Vec3{}
	for i := 0; i < 3; i++ {
		delta[i] = sourceU[i]*(local[0]-0.5) + sourceV[i]*(local[1]-0.5)
	}
	return Vec2{0.5 + dot(delta, worldU), 0.5 + dot(delta, worldV)}
}

func Bake(resolved *model.Resolved, application asset.Apply, state blockstate.State, materials *material.Library) (Mesh, error) {
	result := Mesh{Quads: []Quad{}}
	for _, element := range resolved.Elements {
		lo, hi := Vec3(element.From), Vec3(element.To)
		for _, side := range directions {
			face, ok := element.Faces[asset.Direction(side)]
			if !ok {
				continue
			}
			if math.Mod(face.Rotation, 90) != 0 {
				return Mesh{}, fmt.Errorf("face UV rotation must be a multiple of 90")
			}
			positions := corners(side, lo, hi)
			for i, vertex := range positions {
				v := elementRotate(vertex, element.Rotation)
				v = add(blockRotate(sub(v, Vec3{8, 8, 8}), application), Vec3{8, 8, 8})
				positions[i] = Vec3{v[0] / 16, v[1] / 16, v[2] / 16}
			}
			normal := unit(cross(sub(positions[1], positions[0]), sub(positions[2], positions[0])))
			if normal == (Vec3{}) {
				continue
			}
			shadeNormal := normal
			if element.ShadeDirectionOverride != "" {
				shadeNormal = blockRotate(normals[string(element.ShadeDirectionOverride)], application)
			}
			texture, forceBlend, err := resolved.Texture(face.Texture)
			if err != nil {
				return Mesh{}, err
			}
			unlit := element.Shade != nil && !*element.Shade
			surface, err := materials.Surface(state, texture, forceBlend, unlit)
			if err != nil {
				return Mesh{}, err
			}
			// Holes in a texture do not make the back of a solid element an
			// additional surface. Keep volume faces front-sided; a standalone
			// cutout plane can still be viewed from both sides.
			if surface.Mode == material.Cutout && lo[0] != hi[0] && lo[1] != hi[1] && lo[2] != hi[2] {
				surface.DoubleSided = false
			}
			// A plane with explicit front/back faces already has both sides.
			// Drawing each of those faces double-sided creates coplanar overlap.
			axis := faceAxis[side]
			if lo[axis] == hi[axis] {
				if _, exists := element.Faces[asset.Direction(opposite[side])]; exists {
					surface.DoubleSided = false
				}
			}
			tint := -1
			if face.TintIndex != nil {
				tint = *face.TintIndex
			}
			quad := Quad{Positions: positions, Normal: normal, ShadeNormal: shadeNormal, Color: materials.Tint(state, tint), Surface: surface}
			if face.CullFace != "" {
				quad.CullFace = direction(blockRotate(normals[string(face.CullFace)], application))
			}
			uv := defaultUV(side, lo, hi)
			if face.UV != nil {
				uv = *face.UV
			}
			local := [4]Vec2{{0, 0}, {0, 1}, {1, 1}, {1, 0}}
			steps := (int(face.Rotation/90)%4 + 4) % 4
			for i := 0; i < 4; i++ {
				sample := local[i]
				if application.UVLock {
					sample = lockedUV(sample, side, application)
				}
				for n := 0; n < steps; n++ {
					sample = Vec2{sample[1], 1 - sample[0]}
				}
				quad.UV[i] = Vec2{(uv[0] + sample[0]*(uv[2]-uv[0])) / 16, (uv[1] + sample[1]*(uv[3]-uv[1])) / 16}
			}
			result.Quads = append(result.Quads, quad)
		}
	}
	return result, nil
}
