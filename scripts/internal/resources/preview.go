// Package resources contains explicit static preview choices that are not
// available in the model JSON (biome-independent tints and glass/fluid layers).
package resources

import (
	"strconv"
	"strings"

	"neomega-assets/asset"
	"neomega-viewer/blockstate"
	"neomega-viewer/mesher/material"
	"neomega-viewer/scripts/internal/materials"
	"neomega-viewer/scripts/internal/model"
)

type PreviewPolicy struct{}

func (PreviewPolicy) Mode(state blockstate.State, sampled material.Mode) material.Mode {
	name := state.Name.Path()
	if strings.Contains(name, "glass") || name == "water" || name == "bubble_column" || name == "ice" || name == "honey_block" || name == "slime_block" {
		return material.Blend
	}
	return sampled
}

func (PreviewPolicy) Opacity(state blockstate.State) float32 {
	if state.Name.Path() == "water" || state.Name.Path() == "bubble_column" {
		return 0.7
	}
	return 1
}

// These fixed colors make isolated previews readable. A caller may provide a
// different materials.Policy for another biome or a particular static scene.
func (PreviewPolicy) Tint(state blockstate.State, index int) [3]float32 {
	if index < 0 {
		return [3]float32{1, 1, 1}
	}
	name := state.Name.Path()
	switch {
	case name == "redstone_wire":
		power, _ := strconv.Atoi(state.Properties["power"])
		p := float64(power) / 15
		r := uint32((0.3 + 0.6*p) * 255)
		g := uint32(max(0, p*p*0.7-0.5) * 255)
		b := uint32(max(0, p*p*0.6-0.7) * 255)
		return materials.Linear(r<<16 | g<<8 | b)
	case name == "water" || name == "bubble_column":
		return materials.Linear(0x3f76e4)
	case name == "birch_leaves":
		return materials.Linear(0x80a755)
	case name == "spruce_leaves":
		return materials.Linear(0x619961)
	case strings.HasSuffix(name, "_leaves") || name == "vine":
		return materials.Linear(0x77ab2f)
	default:
		return materials.Linear(0x91bd59)
	}
}

// PreviewProperties supplies the dry-block convention. Fluid layers can be
// represented by static caller overrides when a scene actually contains water.
func PreviewProperties(state blockstate.State) blockstate.State {
	if _, exists := state.Properties["waterlogged"]; !exists {
		state = state.With(map[string]string{"waterlogged": "false"})
	}
	return state
}

// FluidPreview creates an isolated, still fluid surface, without world queries
// or simulation. A section renderer can supply its own neighbor-based surface.
func FluidPreview(state blockstate.State, family string) *model.Resolved {
	fluid := "water"
	if family == "lava" {
		fluid = "lava"
	}
	level, _ := strconv.Atoi(state.Properties["level"])
	height := 8.0 / 9
	if level >= 8 {
		height = 1
	} else {
		height = float64(8-level) / 9
	}
	elements := []asset.Element{{From: asset.Vec3{0, 0, 0}, To: asset.Vec3{16, height * 16, 16}, Faces: map[asset.Direction]asset.Face{}}}
	for _, side := range []asset.Direction{"down", "up", "north", "south", "west", "east"} {
		texture := asset.TextureRef("#flow")
		if side == "down" || side == "up" {
			texture = "#still"
		}
		face := asset.Face{Texture: texture}
		if fluid == "water" {
			index := 0
			face.TintIndex = &index
		}
		elements[0].Faces[side] = face
	}
	if fluid == "lava" {
		shade := false
		elements[0].Shade = &shade
	}
	return &model.Resolved{
		Elements: elements,
		Textures: map[string]asset.TextureBinding{
			"still": {Sprite: asset.TextureRef("minecraft:block/" + fluid + "_still")},
			"flow":  {Sprite: asset.TextureRef("minecraft:block/" + fluid + "_flow")},
		},
	}
}
