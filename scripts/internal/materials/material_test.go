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

func TestAtlasFrameMetadata(t *testing.T) {
	atlas := asset.Atlas{Width: 1, Height: 32, Textures: map[string]asset.AtlasTexture{
		"static":   {SU: 1, SV: 1.0 / 32, Frames: 1},
		"animated": {SU: 1, SV: 1, Frames: 32, FrameTime: 2},
	}}
	library := materials.New(atlas, image.NewNRGBA(image.Rect(0, 0, 1, 32)), resources.PreviewPolicy{})
	for name, entry := range atlas.Textures {
		surface, err := library.Surface(blockstate.State{Name: "minecraft:stone"}, asset.ID("minecraft:block/"+name), false, false)
		if err != nil {
			t.Fatal(err)
		}
		if surface.Frames != entry.Frames || surface.FrameTime != entry.FrameTime || surface.Region != [4]float32{0, 0, 1, float32(entry.SV)} {
			t.Fatalf("atlas metadata changed: %+v", surface)
		}
	}
}
