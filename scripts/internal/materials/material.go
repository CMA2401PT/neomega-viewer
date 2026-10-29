package materials

import (
	"fmt"
	"image"
	"math"

	"neomega-assets/asset"
	"neomega-viewer/blockstate"
	"neomega-viewer/mesher/material"
)

// Policy supplies semantic exceptions and static preview tints. Alpha holes and
// translucency are different from a neighboring block's geometric coverage.
type Policy interface {
	Mode(state blockstate.State, sampled material.Mode) material.Mode
	Tint(state blockstate.State, index int) [3]float32
	Opacity(state blockstate.State) float32
}

type Library struct {
	atlas  asset.Atlas
	modes  map[string]material.Mode
	policy Policy
}

func New(atlas asset.Atlas, pixels image.Image, policy Policy) *Library {
	modes := map[string]material.Mode{}
	for name, texture := range atlas.Textures {
		x := int(math.Round(texture.U * float64(atlas.Width)))
		y := int(math.Round(texture.V * float64(atlas.Height)))
		w := int(math.Round(texture.SU * float64(atlas.Width)))
		h := int(math.Round(texture.SV * float64(atlas.Height)))
		mode := material.Opaque
		for py := y; py < y+h && mode != material.Blend; py++ {
			for px := x; px < x+w; px++ {
				_, _, _, alpha := pixels.At(px, py).RGBA()
				if alpha > 0 && alpha < 65535 {
					mode = material.Blend
					break
				}
				if alpha == 0 {
					mode = material.Cutout
				}
			}
		}
		modes[name] = mode
	}
	return &Library{atlas: atlas, modes: modes, policy: policy}
}

func (library *Library) Surface(state blockstate.State, texture asset.ID, forceBlend, unlit bool) (material.Surface, error) {
	entry, ok := library.atlas.Textures[texture.TextureName()]
	if !ok {
		return material.Surface{}, fmt.Errorf("atlas texture %s missing", texture)
	}
	mode := library.policy.Mode(state, library.modes[texture.TextureName()])
	if forceBlend {
		mode = material.Blend
	}
	return material.Surface{
		Texture:     string(texture),
		Region:      [4]float32{float32(entry.U), float32(entry.V), float32(entry.SU), float32(entry.SV)},
		Mode:        mode,
		DoubleSided: mode != material.Opaque,
		Unlit:       unlit,
		Opacity:     library.policy.Opacity(state),
		Frames:      entry.Frames,
		FrameTime:   entry.FrameTime,
	}, nil
}

func (library *Library) Tint(state blockstate.State, index int) [3]float32 {
	return library.policy.Tint(state, index)
}

// Linear converts an sRGB preview color for multiplication with a linear texture.
func Linear(rgb uint32) [3]float32 {
	var result [3]float32
	for i, shift := range []int{16, 8, 0} {
		v := float64((rgb>>shift)&255) / 255
		if v <= 0.04045 {
			v /= 12.92
		} else {
			v = math.Pow((v+0.055)/1.055, 2.4)
		}
		result[i] = float32(v)
	}
	return result
}
