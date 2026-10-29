package blockstate

import (
	"testing"

	"neomega-blocks/ir"
)

func TestJavaHashesRetainStateProperties(t *testing.T) {
	table, err := Java()
	if err != nil {
		t.Fatal(err)
	}
	if len(table.ByID) != 18254 {
		t.Fatalf("got %d Java states", len(table.ByID))
	}
	for _, check := range []struct {
		State string
		Key   string
		Value string
	}{
		{`minecraft:oak_fence ["west"="false","south"="false","north"="true","east"="false"]`, "north", "true"},
		{`minecraft:oak_fence ["east"="false","north"="false","south"="true","west"="false"]`, "north", "false"},
		{`minecraft:redstone_wire ["west"="none","south"="none","power"="15","north"="none","east"="none"]`, "power", "15"},
		{`minecraft:water ["level"="3"]`, "level", "3"},
	} {
		id := ir.IRBlock(check.State).Tidy().Hash()
		binding, err := table.Resolve(id)
		if err != nil {
			t.Fatal(err)
		}
		if binding.ID != id || binding.Java.Properties[check.Key] != check.Value {
			t.Fatalf("state information lost: %+v", binding)
		}
	}
}

func TestPreviewPropertiesDoNotChangeTheBinding(t *testing.T) {
	source := State{Name: "minecraft:stone"}
	preview := source.With(map[string]string{"example": "first"})
	second := preview.With(map[string]string{"example": "second"})
	if source.Properties != nil || preview.Properties["example"] != "first" || second.Properties["example"] != "second" {
		t.Fatal("preview overrides mutated a previous state")
	}
}
