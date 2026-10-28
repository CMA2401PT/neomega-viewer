package model

import (
	"testing"

	"neomega-assets/asset"
)

func TestVariantsAndMultipartKeepTheSelectionTree(t *testing.T) {
	state := asset.BlockState{Multipart: []asset.Part{
		{Apply: asset.Choice{Options: []asset.Apply{{Model: "post"}}}},
		{When: &asset.Condition{Properties: map[string]string{"north": "true"}}, Apply: asset.Choice{Options: []asset.Apply{{Model: "north"}}}},
		{When: &asset.Condition{OR: []asset.Condition{{Properties: map[string]string{"east": "true"}}, {AND: []asset.Condition{{Properties: map[string]string{"south": "true"}}, {Properties: map[string]string{"west": "false"}}}}}}, Apply: asset.Choice{Options: []asset.Apply{{Model: "rail"}}}},
	}}
	selected, err := Select(state, map[string]string{"north": "true", "east": "false", "south": "true", "west": "false"}, 0)
	if err != nil || len(selected) != 3 {
		t.Fatalf("multipart selection: %v %v", selected, err)
	}
	selected, err = Select(state, map[string]string{"north": "false", "east": "false", "south": "true", "west": "true"}, 0)
	if err != nil || len(selected) != 1 || selected[0].Model != "post" {
		t.Fatalf("unmatched parts were included: %v %v", selected, err)
	}
	variants := asset.BlockState{Variants: map[string]asset.Choice{
		"type=bottom": {Options: []asset.Apply{{Model: "slab"}}},
		"type=top":    {Options: []asset.Apply{{Model: "slab", X: 180, UVLock: true}}},
	}}
	selected, err = Select(variants, map[string]string{"type": "top"}, 0)
	if err != nil || len(selected) != 1 || selected[0].X != 180 || !selected[0].UVLock {
		t.Fatalf("variant transform lost: %v %v", selected, err)
	}
	if _, err := Select(variants, map[string]string{"type": "invalid"}, 0); err == nil {
		t.Fatal("invalid state silently selected a model")
	}
}

func TestWeightsAreRepeatableAndRemainAlternatives(t *testing.T) {
	heavy := 3
	state := asset.BlockState{Variants: map[string]asset.Choice{"": {Array: true, Options: []asset.Apply{{Model: "heavy", Weight: &heavy}, {Model: "light"}}}}}
	count := map[asset.ID]int{}
	for seed := uint64(0); seed < 256; seed++ {
		a, err := Select(state, nil, seed)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := Select(state, nil, seed)
		if len(a) != 1 || a[0].Model != b[0].Model {
			t.Fatal("choice was expanded or changed for the same seed")
		}
		count[a[0].Model]++
	}
	if count["light"] == 0 || count["heavy"] <= count["light"] {
		t.Fatalf("weights not reflected in choices: %v", count)
	}
}

func TestInheritanceAndTextureSlotsDoNotMutateParents(t *testing.T) {
	elements := []asset.Element{{Faces: map[asset.Direction]asset.Face{"up": {Texture: "#alias"}}}}
	empty := []asset.Element{}
	resolver := NewResolver(asset.Models{
		"block/base":  {Elements: &elements, Textures: map[string]asset.TextureBinding{"alias": {Sprite: "#skin"}, "skin": {Sprite: "block/old"}}},
		"block/child": {Parent: "block/base", Textures: map[string]asset.TextureBinding{"skin": {Sprite: "block/new"}}},
		"block/empty": {Parent: "block/child", Elements: &empty},
	})
	child, err := resolver.Resolve("minecraft:block/child")
	if err != nil {
		t.Fatal(err)
	}
	texture, _, err := child.Texture("#alias")
	if err != nil || texture != "minecraft:block/new" || len(child.Elements) != 1 {
		t.Fatalf("child inheritance: %s %v", texture, err)
	}
	parent, _ := resolver.Resolve("minecraft:block/base")
	texture, _, _ = parent.Texture("#alias")
	if texture != "minecraft:block/old" {
		t.Fatal("child changed the parent")
	}
	cleared, _ := resolver.Resolve("minecraft:block/empty")
	if len(cleared.Elements) != 0 {
		t.Fatal("explicit empty elements inherited geometry")
	}
}
