package source

import (
	"context"
	"testing"

	"celler"
	"celler/java"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer-core/viewer/dispatcher"
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

func TestMemorySectionsChunks(t *testing.T) {
	sections := MemorySections{
		{X: -2, Y: -4, Z: 3}: &dispatcher.Section{},
		{X: -2, Y: 8, Z: 3}:  &dispatcher.Section{},
		{X: 5, Y: 0, Z: -1}:  &dispatcher.Section{},
	}
	chunks, err := sections.Chunks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[celler.ChunkPos]bool{{X: -2, Z: 3}: true, {X: 5, Z: -1}: true}
	if len(chunks) != len(want) {
		t.Fatal("不同高度没有合并为水平区块", chunks)
	}
	for _, pos := range chunks {
		if !want[pos] {
			t.Fatal("区块坐标重复或偏移", pos)
		}
		delete(want, pos)
	}
	empty, err := (MemorySections{}).Chunks(context.Background())
	if err != nil || len(empty) != 0 {
		t.Fatal(empty, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sections.Chunks(ctx); err != context.Canceled {
		t.Fatal(err)
	}
}
