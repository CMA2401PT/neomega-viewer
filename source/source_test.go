package source

import (
	"context"
	"testing"

	"celler"
	"celler/java"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer/dispatcher"
)

func TestGroupBlockEntitiesNegativeCoordinates(t *testing.T) {
	entities := []java.BlockEntity{
		{
			Pos: celler.BlockPos{
				X: -17,
				Y: -1,
				Z: 32,
			},
			NBT: nbt.Compound{"id": "example:below"},
		},
		{
			Pos: celler.BlockPos{
				X: -32,
				Y: 0,
				Z: 47,
			},
			NBT: nbt.Compound{"id": "example:above"},
		},
	}
	grouped := GroupBlockEntities(entities)
	if len(grouped) != 2 || grouped[-1][dispatcher.CellPos{15, 15, 0}]["id"] != "example:below" || grouped[0][dispatcher.CellPos{0, 0, 15}]["id"] != "example:above" {
		t.Fatal(grouped)
	}
}

func TestMemorySectionsMissingAndCancelled(t *testing.T) {
	sections := MemorySections{}
	if section, err := sections.Section(context.Background(), dispatcher.SectionPos{}); section != nil || err != nil {
		t.Fatal(section, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sections.Section(ctx, dispatcher.SectionPos{}); err != context.Canceled {
		t.Fatal(err)
	}
}
