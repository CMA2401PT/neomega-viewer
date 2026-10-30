package dispatcher_test

import (
	"slices"
	"testing"

	"neomega-blocks/ir"
	"neomega-viewer/dispatcher"
	"neomega-viewer/fixer"
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
	got := dispatcher.NewJava(routes).Dispatch(blocks, map[[3]uint8]string{pos: snbt}, nil)
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

func TestFixerBeforeActorRemoval(t *testing.T) {
	f, err := fixer.New()
	if err != nil {
		t.Fatal(err)
	}
	blocks := airBlocks()
	fence, actor := 4<<8|4<<4|4, 5<<8|4<<4|4
	blocks[fence] = hash(`minecraft:oak_fence ["east"="false","north"="false","south"="false","west"="false"]`)
	// 陶罐侧面不满格，栅栏本就不连接；用被认领为 actor 的石头显现移除顺序的差别。
	for _, name := range []string{"decorated_pot", "stone"} {
		state := "minecraft:stone []"
		if name == "decorated_pot" {
			state = `minecraft:decorated_pot ["cracked"="false","facing"="north"]`
		}
		blocks[actor] = hash(state)
		called := false
		got := dispatcher.NewJava(dispatcher.JavaRoutes(nil, []string{"minecraft:" + name})).Dispatch(
			blocks,
			nil,
			func(layer []uint32) []uint32 {
				called = true
				if layer[actor] != blocks[actor] {
					t.Fatal("fixer saw removed actor")
				}
				return f.Fix(layer, fixer.Neighbours{})
			},
		)
		want := blocks[fence]
		if name == "stone" {
			want = hash(`minecraft:oak_fence ["east"="true","north"="false","south"="false","west"="false"]`)
		}
		if !called || got.Layers[0][fence] != want || got.Layers[0][actor] != hash("minecraft:air []") {
			t.Fatalf("%s: fixer did not preserve original neighbor semantics", name)
		}
		if name == "stone" && f.Fix(got.Layers[0], fixer.Neighbours{})[fence] == want {
			t.Fatal("example does not distinguish fixing after removal")
		}
	}
}
