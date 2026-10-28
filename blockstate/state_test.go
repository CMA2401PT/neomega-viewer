package blockstate

import "testing"

func TestPreviewPropertiesDoNotChangeTheBinding(t *testing.T) {
	source := State{Name: "minecraft:stone"}
	preview := source.With(map[string]string{"example": "first"})
	second := preview.With(map[string]string{"example": "second"})
	if source.Properties != nil || preview.Properties["example"] != "first" || second.Properties["example"] != "second" {
		t.Fatal("preview overrides mutated a previous state")
	}
}
