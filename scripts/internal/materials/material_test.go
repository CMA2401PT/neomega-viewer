package materials_test

import (
	"image"
	"image/color"
	"testing"

	"neomega-assets/asset"
	"neomega-viewer/blockstate"
	"neomega-viewer/mesher/material"
	"neomega-viewer/scripts/internal/materials"
	"neomega-viewer/scripts/internal/resources"
)

func TestAlphaHolesAndTranslucencyAreDifferent(t *testing.T) {
	pixels := image.NewNRGBA(image.Rect(0, 0, 6, 1))
	for x, alpha := range []uint8{255, 255, 0, 255, 128, 255} {
		pixels.SetNRGBA(x, 0, color.NRGBA{R: 255, A: alpha})
	}
	atlas := asset.Atlas{Width: 6, Height: 1, Textures: map[string]asset.AtlasTexture{
		"solid": {U: 0, SU: 2.0 / 6, SV: 1}, "holes": {U: 2.0 / 6, SU: 2.0 / 6, SV: 1}, "partial": {U: 4.0 / 6, SU: 2.0 / 6, SV: 1},
	}}
	library := materials.New(atlas, pixels, resources.PreviewPolicy{})
	for texture, want := range map[asset.ID]material.Mode{"minecraft:block/solid": material.Opaque, "minecraft:block/holes": material.Cutout, "minecraft:block/partial": material.Blend} {
		surface, err := library.Surface(blockstate.State{Name: "minecraft:stone"}, texture, false, false)
		if err != nil || surface.Mode != want {
			t.Fatalf("%s: %+v %v", texture, surface, err)
		}
	}
	glass, err := library.Surface(blockstate.State{Name: "minecraft:glass"}, "minecraft:block/holes", false, false)
	if err != nil || glass.Mode != material.Blend || !glass.DoubleSided {
		t.Fatal("glass lost its transparent surface semantics")
	}
}
