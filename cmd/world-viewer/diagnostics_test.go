package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"reflect"
	"testing"

	"celler"
	"celler/java"
	"celler/layer_storage"
	"neomega-blocks/ir"
	preset "neomega-blocks/presets/java/v1/geometry"
	"neomega-viewer/merger"
)

func TestColumnMissingModelDiagnostics(t *testing.T) {
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(resources.Tables, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Unknown states remain diagnostic; modern and legacy grass are renderable.
	// Air intentionally has no geometry and must not be reported as missing.
	missing := ir.IRBlock("example:missing []")
	legacy := engine.prepare(&java.Section{Blocks: layer_storage.MakePaletteStorage(ir.IRBlock("minecraft:grass []"))})
	modern := engine.prepare(&java.Section{Blocks: layer_storage.MakePaletteStorage(ir.IRBlock("minecraft:short_grass []"))})
	air := engine.prepare(&java.Section{Blocks: layer_storage.MakePaletteStorage(ir.IRBlock("minecraft:air []"))})
	if legacy.unknown != 0 || legacy.empty || modern.unknown != 0 || modern.empty || len(air.issues) != 0 || !air.empty {
		t.Fatal("valid short grass or intentionally invisible air reported as missing")
	}
	root := t.TempDir()
	world, err := java.OpenDirectory(root, java.Options{})
	if err != nil {
		t.Fatal(err)
	}
	chunk := java.NewChunk(celler.ChunkPos{X: -2, Z: 14}, 3465)
	for _, y := range []int32{-1, 0} {
		blocks := layer_storage.MakePaletteStorage(ir.IRBlock("minecraft:air []"))
		blocks.Set(layer_storage.At(8, 3, 2), missing)
		blocks.Set(layer_storage.At(9, 3, 2), missing)
		if err = chunk.SetSection(&java.Section{Y: y, Blocks: blocks}); err != nil {
			t.Fatal(err)
		}
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
	packet, err := engine.BuildColumn(context.Background(), source, chunkKey{X: -2, Z: 14, MinY: -1, MaxY: 0})
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(packet[4:]) != 3 || binary.LittleEndian.Uint32(packet[12:]) != 4 {
		t.Fatal("wrong protocol or missing block count")
	}
	offset := 20
	markers := 0
	for range binary.LittleEndian.Uint32(packet[8:]) {
		size := int(binary.LittleEndian.Uint32(packet[offset+4:]))
		frame, err := merger.Unmarshal(bytes.NewReader(packet[offset+12 : offset+12+size]))
		if err != nil {
			t.Fatal(err)
		}
		for _, quad := range frame.Mesh.Quads {
			if quad.Color == [3]float32{1, 0, 1} {
				markers++
			}
		}
		offset += 12 + size
	}
	if markers == 0 {
		t.Fatal("unknown state disappeared instead of using missing-model markers")
	}
	if offset+int(binary.LittleEndian.Uint32(packet[16:])) != len(packet) {
		t.Fatal("wrong diagnostics length")
	}
	var issues []RenderIssue
	if err = json.Unmarshal(packet[offset:], &issues); err != nil {
		t.Fatal(err)
	}
	want := []RenderIssue{{Position: [3]int32{-24, -13, 226}, Blockstate: string(missing), Reason: "missing_model"}}
	if !reflect.DeepEqual(issues, want) {
		t.Fatalf("diagnostics: %+v, want %+v", issues, want)
	}
	// Splitting a waterlogged state must not strip properties from its report.
	original := ir.IRBlock(`example:missing ["waterlogged"="true"]`)
	prepared := engine.prepare(&java.Section{Y: -1, Blocks: layer_storage.MakePaletteStorage(original)})
	if len(prepared.issues) != 1 || prepared.issues[0].Blockstate != string(original) {
		t.Fatal("source blockstate lost")
	}
}
