package dispatcher_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"celler/layer_storage"
	blockgeometry "neomega-blocks/geometry"
	"neomega-blocks/ir"
	resources "neomega-blocks/presets/java/v1/geometry"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer/blockactor"
	"neomega-viewer/dispatcher"
	"neomega-viewer/effector"
	"neomega-viewer/mesher/geometry"
)

func section() dispatcher.Section {
	return dispatcher.Section{Blocks: layer_storage.MakePaletteStorage(ir.IRBlock("minecraft:air []"))}
}

func tables(t *testing.T) *blockgeometry.Tables[uint32] {
	t.Helper()
	resources, err := resources.Open(resources.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return resources.Tables
}

type actorHandler struct {
	inputs []blockactor.Input
	names  []string
}

func (h *actorHandler) BlockNames() []string {
	if h.names != nil {
		return h.names
	}
	return []string{"minecraft:decorated_pot"}
}
func (h *actorHandler) Build(input blockactor.Input) (geometry.Mesh, error) {
	h.inputs = append(h.inputs, input)
	if input.NBT["fail"] == true {
		return geometry.Mesh{}, errors.New("appearance failed")
	}
	return geometry.Mesh{Quads: []geometry.Quad{{Positions: [4]geometry.Vec3{{0, 0, 0}}}}}, nil
}

func TestWaterloggedActorAndEffect(t *testing.T) {
	handler := &actorHandler{}
	pipeline := dispatcher.NewPipeline(
		tables(t),
		effector.New(map[string]string{"minecraft:enchanting_table": "enchanting_table_book"}),
		blockactor.New(handler),
		nil,
	)
	input := section()
	pot := ir.IRBlock(`minecraft:decorated_pot ["cracked"="false","facing"="east","waterlogged"="true"]`)
	futurePot := ir.IRBlock(pot.NameWithNameSpace() + " " + string(pot.States().Set("future", "yes"))).Tidy()
	input.Blocks.Set(layer_storage.At(4, 5, 6), futurePot)
	input.Blocks.Set(layer_storage.At(8, 5, 6), pot)
	input.Blocks.Set(layer_storage.At(1, 2, 3), ir.IRBlock("minecraft:enchanting_table []"))
	input.Entities = map[dispatcher.CellPos]nbt.Compound{
		{4, 5, 6}: {"sherds": []string{"minecraft:brick"}},
		{8, 5, 6}: {"fail": true},
	}
	before := input.Blocks.GetUsedPalette()
	result, err := pipeline.Build(input, dispatcher.Neighbours{}, dispatcher.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Frame.Actors) != 1 || result.Frame.Actors[0].Pos != (dispatcher.CellPos{4, 5, 6}) || result.Frame.Actors[0].Mesh.Quads[0].Positions[0] != (geometry.Vec3{4, 5, 6}) {
		t.Fatal("successful actor or translation lost", result.Frame.Actors)
	}
	if len(handler.inputs) != 2 || !handler.inputs[0].Waterlogged || handler.inputs[0].State != futurePot || !reflect.DeepEqual(handler.inputs[0].NBT, input.Entities[dispatcher.CellPos{4, 5, 6}]) {
		t.Fatal("complete actor input lost", handler.inputs)
	}
	if len(result.Frame.Effects) != 1 || result.Frame.Effects[0].Effect != "enchanting_table_book" {
		t.Fatal(result.Frame.Effects)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Kind != dispatcher.ActorError || result.Diagnostics[0].Pos != (dispatcher.CellPos{8, 5, 6}) || result.Diagnostics[0].State != pot || result.Diagnostics[0].Err == nil {
		t.Fatal(result.Diagnostics)
	}
	water, failedBody, successfulBody, table := false, false, false, false
	for _, quad := range result.Frame.Mesh.Quads {
		texture := quad.Surface.Texture
		water = water || texture == "minecraft:block/water_still"
		table = table || texture == "minecraft:block/enchanting_table_top"
		if strings.Contains(texture, "decorated_pot") {
			failedBody = failedBody || quad.Positions[0][0] >= 8
			successfulBody = successfulBody || quad.Positions[0][0] < 8
		}
	}
	if !water || !failedBody || successfulBody || !table {
		t.Fatalf("water=%v failedBody=%v successfulBody=%v table=%v", water, failedBody, successfulBody, table)
	}
	if !reflect.DeepEqual(before, input.Blocks.GetUsedPalette()) || input.Blocks.Get(layer_storage.At(4, 5, 6)) != futurePot {
		t.Fatal("pipeline changed caller input")
	}
}

func TestMissingDiagnosticsPreserveState(t *testing.T) {
	missing := &blockgeometry.Shape{Quads: []geometry.Quad{{Color: [3]float32{1, 0, 1}}}}
	pipeline := dispatcher.NewJavaPipeline(&blockgeometry.Tables[uint32]{
		Index: map[uint32][]blockgeometry.ModelID{ir.IRBlock("minecraft:air []").Hash(): {}},
	}, nil, missing)
	input := section()
	state := ir.IRBlock(`example:unknown ["waterlogged"="true"]`)
	input.Blocks.Set(layer_storage.At(4, 5, 6), state)
	neighbour := section()
	neighbour.Blocks.Set(layer_storage.At(15, 0, 0), ir.IRBlock("example:neighbour []"))
	result, err := pipeline.Build(input, dispatcher.Neighbours{&neighbour}, dispatcher.Options{})
	if err != nil || len(result.Diagnostics) != 1 {
		t.Fatalf("missing report: %v %v", result, err)
	}
	want := dispatcher.Diagnostic{Kind: dispatcher.MissingModel, Pos: dispatcher.CellPos{4, 5, 6}, State: state}
	if result.Diagnostics[0] != want {
		t.Fatal(result.Diagnostics)
	}
	strict := dispatcher.NewJavaPipeline(tables(t), nil, nil)
	if _, err := strict.Build(input, dispatcher.Neighbours{}, dispatcher.Options{}); err == nil {
		t.Fatal("missing shape nil did not return error")
	}
}

type source struct {
	sections map[dispatcher.SectionPos]*dispatcher.Section
	calls    map[dispatcher.SectionPos]int
	err      error
}

func (s *source) Section(ctx context.Context, pos dispatcher.SectionPos) (*dispatcher.Section, error) {
	s.calls[pos]++
	return s.sections[pos], s.err
}

func TestBuildSectionsCachesNeighbours(t *testing.T) {
	pipeline := dispatcher.NewJavaPipeline(tables(t), nil, nil)
	s := &source{
		sections: map[dispatcher.SectionPos]*dispatcher.Section{},
		calls:    map[dispatcher.SectionPos]int{},
	}
	positions := []dispatcher.SectionPos{{X: -2, Y: 3, Z: 4}, {X: -1, Y: 3, Z: 4}, {X: 10}}
	for _, pos := range positions[:2] {
		value := section()
		value.Blocks = layer_storage.MakePaletteStorage(ir.IRBlock("minecraft:stone []"))
		s.sections[pos] = &value
	}
	results, err := pipeline.BuildSections(context.Background(), s, positions, dispatcher.Options{})
	if err != nil || len(results) != 3 {
		t.Fatalf("batch: %v %v", results, err)
	}
	for i, result := range results {
		if result.Pos != positions[i] || (i < 2 && result.Result.Stats.VisibleQuads != 1280) || (i == 2 && len(result.Result.Frame.Mesh.Quads) != 0) {
			t.Fatal("neighbour culling or output position changed", result)
		}
	}
	for pos, calls := range s.calls {
		if calls != 1 {
			t.Fatalf("%v loaded %d times", pos, calls)
		}
	}
	s.err = errors.New("source failed")
	if _, err := pipeline.BuildSections(context.Background(), s, positions, dispatcher.Options{}); !errors.Is(err, s.err) {
		t.Fatal("source error lost", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := pipeline.BuildSections(ctx, s, positions, dispatcher.Options{}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}

func TestActorNeighboursDoNotOcclude(t *testing.T) {
	handler := &actorHandler{names: []string{"minecraft:stone"}}
	pipeline := dispatcher.NewPipeline(tables(t), effector.New(nil), blockactor.New(handler), nil)
	input := section()
	input.Blocks.Set(layer_storage.At(0, 0, 0), ir.IRBlock("minecraft:dirt []"))
	neighbour := section()
	neighbour.Blocks.Set(layer_storage.At(15, 0, 0), ir.IRBlock(`minecraft:stone ["waterlogged"="true"]`))
	result, err := pipeline.Build(input, dispatcher.Neighbours{&neighbour}, dispatcher.Options{})
	if err != nil || result.Stats.OutputQuads != 6 || len(handler.inputs) != 0 || len(result.Diagnostics) != 0 {
		t.Fatalf("neighbour actor built or hid visible faces: %+v %v", result, err)
	}
}

func TestBlocksFromHashes(t *testing.T) {
	state := ir.IRBlock(`minecraft:oak_slab ["type"="bottom","waterlogged"="true"]`)
	blocks := make([]uint32, 4096)
	for i := range blocks {
		blocks[i] = ir.IRBlock("minecraft:air []").Hash()
	}
	blocks[4<<8|6<<4|5] = state.Hash()
	palette, err := dispatcher.BlocksFromHashes(blocks)
	if err != nil || palette.Get(layer_storage.At(4, 5, 6)) != state {
		t.Fatal("complete state not restored", err)
	}
	blocks[0] = ir.IRBlock("example:unknown []").Hash()
	if _, err := dispatcher.BlocksFromHashes(blocks); err == nil {
		t.Fatal("unknown hash accepted")
	}
}
