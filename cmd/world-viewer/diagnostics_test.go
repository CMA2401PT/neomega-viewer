package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"celler"
	"celler/java"
	"celler/layer_storage"
	"neomega-blocks/ir"
	"neomega-viewer-core/viewer/blockactor"
	"neomega-viewer-core/viewer/blockdata"
	"neomega-viewer-core/viewer/cube"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer-core/viewer/effector"
	"neomega-viewer-core/viewer/merger"
	preset "neomega-viewer-core/viewer/resources"
	"neomega-viewer/column"
)

func TestColumnMissingModelDiagnostics(t *testing.T) {
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := newPipeline(resources)
	if err != nil {
		t.Fatal(err)
	}
	// Unknown states remain diagnostic; modern and legacy grass are renderable.
	// Air intentionally has no geometry and must not be reported as missing.
	missing := ir.IRBlock("example:missing []")
	for _, state := range []ir.IRBlock{"minecraft:grass []", "minecraft:short_grass []", "minecraft:air []"} {
		result, err := engine.Build(
			dispatcher.Section{Blocks: blockdata.Uniform(4096, state)},
			dispatcher.Neighbours{},
			dispatcher.Options{},
		)
		if err != nil || len(result.Diagnostics) != 0 {
			t.Fatalf("valid state %s reported as missing: %v, %v", state, result, err)
		}
		if (len(result.Frame.Mesh.Quads) == 0) != (state == "minecraft:air []") {
			t.Fatal("valid short grass or intentionally invisible air changed")
		}
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
	packet, err := column.BuildColumn(
		context.Background(),
		engine,
		source.Reader(),
		column.Key{
			X:    -2,
			Z:    14,
			MinY: -1,
			MaxY: 0,
		},
	)
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
	var issues []column.Issue
	if err = json.Unmarshal(packet[offset:], &issues); err != nil {
		t.Fatal(err)
	}
	want := []column.Issue{{
		Scope:      "block",
		Count:      1,
		Origin:     "render",
		Position:   [3]int32{-24, -13, 226},
		Blockstate: string(missing),
		Reason:     "missing_model",
	}}
	if !reflect.DeepEqual(issues, want) {
		t.Fatalf("diagnostics: %+v, want %+v", issues, want)
	}
	// 同一状态可以同时缺模型和构建 actor 失败，响应中要分别保留两种原因。
	stone, err := resources.Tables.ModelIDs(ir.IRBlock("minecraft:stone []").Hash())
	if err != nil {
		t.Fatal(err)
	}
	failingEngine := dispatcher.NewPipeline(
		cube.New(16),
		resources.Tables,
		effector.New(nil),
		blockactor.New(failedDiagnosticActor{}), nil,
		&resources.Tables.Shapes[stone[0]],
	)
	packet, err = column.BuildColumn(
		context.Background(),
		failingEngine,
		source.Reader(),
		column.Key{
			X:    -2,
			Z:    14,
			MinY: -1,
			MaxY: 0,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	diagnosticSize := int(binary.LittleEndian.Uint32(packet[16:]))
	if err = json.Unmarshal(packet[len(packet)-diagnosticSize:], &issues); err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 || issues[0].Reason != "actor_error" || issues[1].Reason != "missing_model" {
		t.Fatalf("different reasons swallowed each other: %+v", issues)
	}
	for _, issue := range issues {
		if issue.Blockstate != string(missing) || issue.Position != want[0].Position {
			t.Fatal(issue)
		}
	}
	// Splitting a waterlogged state must not strip properties from its report.
	original := ir.IRBlock(`example:missing ["waterlogged"="true"]`)
	result, err := engine.Build(
		dispatcher.Section{Blocks: blockdata.Uniform(4096, original)},
		dispatcher.Neighbours{},
		dispatcher.Options{},
	)
	if err != nil || len(result.Diagnostics) != 4096 {
		t.Fatalf("missing cells: %v, %v", result, err)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.State != original {
			t.Fatal("source blockstate lost")
		}
	}
}

type failedDiagnosticActor struct{}

func (failedDiagnosticActor) BlockNames() []string { return []string{"example:missing"} }
func (failedDiagnosticActor) Build(input blockactor.Input) (blockactor.Appearance, error) {
	return blockactor.Appearance{}, errors.New("actor appearance unavailable")
}
