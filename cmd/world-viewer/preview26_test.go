package main

import (
	"context"
	"strings"
	"testing"

	"celler"
	"celler/bedrock"
	"celler/layer_storage"
	"neomega-blocks/ir"
	current "neomega-blocks/presets/java/v26_3"
	"neomega-converter-core/be2je"
	"neomega-converter-core/be2je/adapt"
	"neomega-viewer-core/viewer/splitlayer"
)

func TestPreview26PoplarHangingSigns(t *testing.T) {
	catalog, err := current.Open()
	if err != nil {
		t.Fatal(err)
	}
	fallback, err := preview26Mapping()
	if err != nil {
		t.Fatal(err)
	}
	for state := range catalog.States() {
		if !strings.HasPrefix(state.Java.ShortName(), "poplar_") || !strings.HasSuffix(state.Java.ShortName(), "hanging_sign") {
			continue
		}
		chunk := bedrock.NewChunk(celler.ChunkPos{}, 0)
		layer := bedrock.NewLayer(bedrock.Block{State: "minecraft:air []"})
		at := layer_storage.At(1, 1, 1)
		source := state.Bedrock
		if source.States().CheckKey("hanging", true) {
			// Bedrock 保留 facing_direction，天花板悬挂牌用 ground_sign_direction 旋转。
			// Geyser 正向表只列一组 facing，不能把其他有效组合当作未知方块。
			source = ir.IRBlock(source.NameWithNameSpace() + " " + string(source.States().Set("facing_direction", int32(0)))).Tidy()
		}
		layer.Blocks.Set(at, bedrock.Block{State: source, Version: 18168865})
		if err := chunk.SetSection(&bedrock.Section{Y: 0, Version: 9, Layers: []*bedrock.Layer{layer}}); err != nil {
			t.Fatal(err)
		}
		converted, err := be2je.ConvertChunkWithStateFallback(chunk, nil, fallback)
		if err != nil {
			t.Fatal(err)
		}
		got := converted.Sections[0].Blocks.Get(at)
		if splitlayer.SplitBlock(got)[0] != splitlayer.SplitBlock(state.Java)[0] || len(converted.Diagnostics) != 0 {
			t.Fatalf("%s -> %s, want %s; diagnostics: %+v", source, got, state.Java, converted.Diagnostics)
		}
	}
}

func TestPreview26BedrockFallbackKeepsExportSeparate(t *testing.T) {
	catalog, err := current.Open()
	if err != nil {
		t.Fatal(err)
	}
	fallback, err := preview26Mapping()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"minecraft:poplar_planks", "minecraft:sulfur", "minecraft:cinnabar", "minecraft:copper_chest", "minecraft:white_wool_stairs"} {
		t.Run(name, func(t *testing.T) {
			definition, ok := catalog.Definition(name)
			if !ok {
				t.Fatal(name)
			}
			state, _ := catalog.ByID(definition.DefaultStateID)
			chunk := bedrock.NewChunk(celler.ChunkPos{}, 0)
			layer := bedrock.NewLayer(bedrock.Block{State: "minecraft:air []"})
			at := layer_storage.At(1, 1, 1)
			layer.Blocks.Set(at, bedrock.Block{State: state.Bedrock, Version: 18168865})
			if err := chunk.SetSection(&bedrock.Section{Y: 0, Version: 9, Layers: []*bedrock.Layer{layer}}); err != nil {
				t.Fatal(err)
			}
			original, err := be2je.ConvertChunk(chunk)
			if err != nil {
				t.Fatal(err)
			}
			preview := adapt.NewConverted(func(context.Context, celler.ChunkPos) (*bedrock.Chunk, error) { return chunk, nil }, adapt.Options{StateFallback: fallback})
			converted, err := preview.Chunk(context.Background(), celler.ChunkPos{})
			if err != nil {
				t.Fatal(err)
			}
			got := converted.Sections[0].Blocks.Get(at)
			if splitlayer.SplitBlock(got)[0] != splitlayer.SplitBlock(state.Java)[0] {
				t.Fatalf("got %s, want %s", got, state.Java)
			}
			if original.Sections[0].Blocks.Get(at).ShortName() != "air" {
				t.Fatal("fixture must exercise unknown legacy state")
			}
			if layer.Blocks.Get(at).State != state.Bedrock {
				t.Fatal("source palette changed")
			}
		})
	}
}
