package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io/fs"
	"testing"

	"celler"
	"celler/java"
	"celler/layer_storage"
	jmodel "neomega-blocks/geometry/java_block_model"
	"neomega-blocks/ir"
	preset "neomega-blocks/presets/java/v1/geometry"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer/column"
	"neomega-viewer/merger"
	"neomega-viewer/viewassets"
)

func TestColumnEntityFailuresKeepTerrain(t *testing.T) {
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := fs.ReadFile(viewassets.Resources(), "blocksAtlases.json")
	if err != nil {
		t.Fatal(err)
	}
	var atlas jmodel.Atlas
	if err = json.Unmarshal(data, &atlas); err != nil {
		t.Fatal(err)
	}
	engine, err := newPipeline(resources.Tables, &atlas)
	if err != nil {
		t.Fatal(err)
	}
	pot := ir.IRBlock(`minecraft:decorated_pot ["cracked"="false","facing"="north","waterlogged"="true"]`)
	for _, tc := range []struct {
		name     string
		entities any
		reason   string
		actors   int
	}{
		{name: "empty_byte_list", entities: []byte{}, actors: 2},
		{name: "invalid_list", entities: []byte{1}, reason: "invalid_block_entities", actors: 2},
		{name: "invalid_position", entities: []nbt.Compound{{"id": "minecraft:decorated_pot", "x": "bad"}}, reason: "invalid_block_entity", actors: 2},
		{name: "invalid_appearance", entities: []nbt.Compound{
			{"id": "minecraft:decorated_pot", "x": int32(4), "y": int32(65), "z": int32(-14), "sherds": []string{"minecraft:unknown_pottery_sherd"}},
		}, reason: "actor_error", actors: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			world, err := java.OpenDirectory(root, java.Options{})
			if err != nil {
				t.Fatal(err)
			}
			chunk := java.NewChunk(celler.ChunkPos{X: 0, Z: -1}, 3465)
			chunk.NBT.Value["block_entities"] = tc.entities
			blocks := layer_storage.MakePaletteStorage(ir.IRBlock("minecraft:air []"))
			blocks.Set(layer_storage.At(0, 0, 0), ir.IRBlock("minecraft:stone []"))
			blocks.Set(layer_storage.At(2, 1, 2), pot)
			blocks.Set(layer_storage.At(4, 1, 2), pot)
			if err = chunk.SetSection(&java.Section{Y: 4, Blocks: blocks}); err != nil {
				t.Fatal(err)
			}
			if err = world.WriteChunk(java.Overworld, chunk); err != nil {
				t.Fatal(err)
			}
			if err = world.Commit(); err != nil {
				t.Fatal(err)
			}
			if err = world.Close(); err != nil {
				t.Fatal(err)
			}
			source, err := OpenSource(SourceOptions{Directory: root})
			if err != nil {
				t.Fatal(err)
			}
			defer source.Close()
			packet, err := column.BuildColumn(
				context.Background(),
				engine,
				source.Reader(),
				column.Key{
					X:    0,
					Z:    -1,
					MinY: 4,
					MaxY: 4,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if binary.LittleEndian.Uint32(packet[8:]) != 1 || binary.LittleEndian.Uint32(packet[12:]) != 0 {
				t.Fatal("lost section or created unknown blocks")
			}
			size := int(binary.LittleEndian.Uint32(packet[24:]))
			frame, err := merger.Unmarshal(bytes.NewReader(packet[32 : 32+size]))
			if err != nil {
				t.Fatal(err)
			}
			if len(frame.Actors) != tc.actors {
				t.Fatalf("actors=%d, want %d", len(frame.Actors), tc.actors)
			}
			// The failed pot keeps actual static geometry at its position, as well as water.
			if tc.reason == "actor_error" {
				found := false
				for _, q := range frame.Mesh.Quads {
					if q.Positions[0][0] >= 4 && q.Positions[0][0] <= 5 && q.Surface.Texture != "minecraft:block/water_still" && q.Surface.Texture != "minecraft:block/water_flow" {
						found = true
					}
				}
				if !found {
					t.Fatal("failed pot lost its static geometry")
				}
			}
			var issues []column.Issue
			if binary.LittleEndian.Uint32(packet[16:]) > 0 {
				if err = json.Unmarshal(packet[32+size:], &issues); err != nil {
					t.Fatal(err)
				}
			}
			if tc.reason == "" {
				if len(issues) != 0 {
					t.Fatal(issues)
				}
				return
			}
			if len(issues) != 1 || issues[0].Reason != tc.reason || issues[0].Detail == "" {
				t.Fatalf("issues=%+v", issues)
			}
			if tc.reason == "actor_error" && (issues[0].Blockstate != string(pot) || issues[0].Position != [3]int32{4, 65, -14}) {
				t.Fatal(issues)
			}
		})
	}
}
