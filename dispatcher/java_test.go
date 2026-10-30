package dispatcher_test

import (
	"slices"
	"testing"

	"neomega-blocks/ir"
	"neomega-viewer/dispatcher"
)

func hash(state string) uint32 { return ir.IRBlock(state).Tidy().Hash() }

func airBlocks() []uint32 {
	blocks := make([]uint32, 4096)
	for i := range blocks {
		blocks[i] = hash("minecraft:air []")
	}
	return blocks
}

func TestWaterloggedActorAndEffect(t *testing.T) {
	routes := dispatcher.JavaRoutes(
		[]string{"minecraft:enchanting_table"},
		[]string{"minecraft:decorated_pot"},
	)
	pot := `minecraft:decorated_pot ["cracked"="false","facing"="east","waterlogged"="true"]`
	for _, wet := range []string{"true", "false"} {
		state := ir.IRBlock(`minecraft:decorated_pot ["cracked"="true","facing"="west","waterlogged"="` + wet + `"]`).Tidy()
		if route, found := routes[state.Hash()]; !found || route.State != state || route.Target != dispatcher.BlockActor {
			t.Fatalf("missing complete route for %s: %+v", state, route)
		}
	}
	blocks := airBlocks()
	pos := [3]uint8{4, 5, 6}
	index := 4<<8 | 6<<4 | 5
	blocks[index] = hash(pot)
	blocks[123] = hash("minecraft:enchanting_table []")
	before := slices.Clone(blocks)
	snbt := `{sherds:["minecraft:brick"]}`
	got := dispatcher.NewJava(routes).Dispatch(blocks, map[[3]uint8]string{pos: snbt})
	if len(got.Layers) != 2 || got.Layers[0][index] != hash("minecraft:air []") || got.Layers[1][index] != hash(`minecraft:water ["level"="0"]`) {
		t.Fatal("actor removal discarded water or retained body")
	}
	if got.Layers[0][123] != blocks[123] || len(got.Effects) != 1 {
		t.Fatal("enchanting table body was removed")
	}
	if len(got.Actors) != 1 || got.Actors[0].Pos != pos || got.Actors[0].SNBT != snbt || !got.Actors[0].Waterlogged || got.Actors[0].State != ir.IRBlock(pot).Tidy() {
		t.Fatalf("actor input: %+v", got.Actors)
	}
	if !slices.Equal(blocks, before) {
		t.Fatal("dispatcher changed caller input")
	}
}
