package main

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"io"
	"io/fs"
	preset "neomega-viewer-core/viewer/resources"
	"neomega-viewer/column"
	"os"
	"path/filepath"
	"testing"

	"celler"
	cellerjava "celler/java"
	"celler/layer_storage"
	"neomega-blocks/ir"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer-core/viewer/dispatcher"
)

func TestStructureFileBecomesJavaWorld(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "tiny.schem")
	root := map[string]any{
		"Version": int32(2),
		"Width":   int16(2),
		"Height":  int16(1),
		"Length":  int16(1),
		"Palette": map[string]any{
			"minecraft:air":   int32(0),
			"minecraft:stone": int32(1),
		},
		"BlockData": nbt.Array([]byte{0x00, 0x01}),
	}
	raw, err := nbt.EncodeToNBTBytes(root, nbt.OptEncodeToNBT{Encoding: nbt.EncodingBigEndian})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	options, err := sourceAtPath(path)
	if err != nil || options.File != path || options.Kind != "schem" {
		t.Fatal(options, err)
	}
	opened, err := OpenSource(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if opened.Info.Kind != "schem" || opened.Info.Path != path || opened.Info.Name != "tiny" {
		t.Fatalf("%+v", opened.Info)
	}
	if opened.Info.Position != [3]float64{1, 15, 0} {
		t.Fatal(opened.Info.Position)
	}
	section, err := opened.Reader().Section(context.Background(), dispatcher.SectionPos{})
	if err != nil || section == nil || section.Blocks == nil {
		t.Fatal(section, err)
	}
	world, err := cellerjava.OpenDirectory(filepath.Join(directory, "tiny"), cellerjava.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer world.Close()
	chunk, err := world.ReadChunk(cellerjava.Overworld, celler.ChunkPos{})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := chunk.Section(0)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Blocks.Get(layer_storage.At(1, 0, 0)) != ir.IRBlock("minecraft:stone []") {
		t.Fatal(stored.Blocks.Get(layer_storage.At(1, 0, 0)))
	}
}

// 真实写入 LevelDB 后再打开，确认原生双层与独立 actor 记录都进入预览来源。
func TestNativeStructureAndMCWorldPreview(t *testing.T) {
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := newPipeline(resources)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "native.mcstructure")
	root := map[string]any{"format_version": int32(1), "structure_world_origin": []int32{16, 0, 0}, "size": []int32{1, 1, 1}, "structure": nbt.Compound{
		"block_indices": []any{[]int32{0}, []int32{-1}},
		"palette":       nbt.Compound{"default": nbt.Compound{"block_palette": []nbt.Compound{{"name": "minecraft:stone", "states": nbt.Compound{}, "version": int32(18168865)}}}},
		"entities":      []nbt.Compound{{"identifier": "minecraft:pig", "Pos": []float32{0.5, 1, 0.5}, "Rotation": []float32{0, 0}}},
	}}
	raw, err := nbt.EncodeToNBTBytes(root, nbt.OptEncodeToNBT{Encoding: nbt.EncodingLittleEndian})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	worldDir := filepath.Join(directory, "native")
	if err = os.Mkdir(worldDir, 0755); err != nil {
		t.Fatal(err)
	}
	check := func(file string) {
		t.Helper()
		options, err := sourceAtPath(file)
		if err != nil {
			t.Fatal(err)
		}
		options.SkipBedrockFix = true
		opened, err := OpenSource(context.Background(), options)
		if err != nil {
			t.Fatal(err)
		}
		defer opened.Close()
		section, err := opened.Reader().Section(context.Background(), dispatcher.SectionPos{X: 1})
		if err != nil || section == nil || section.Blocks == nil || section.Entities == nil || len(section.Entities.List) != 1 {
			t.Fatal(section, err)
		}
		packet, err := column.BuildColumn(context.Background(), pipeline, opened.Reader(), column.Key{X: 1})
		if err != nil || len(packet) < 20 || binary.LittleEndian.Uint32(packet[8:]) != 1 || binary.LittleEndian.Uint32(packet[12:]) != 0 {
			t.Fatal("原生结构未生成预期网格", len(packet), err)
		}
		if opened.Info.Path != file {
			t.Fatal(opened.Info)
		}
	}
	check(path)
	// 已有世界应被复用，源文件后来损坏也不能触发覆盖。
	if err = os.WriteFile(path, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}
	check(path)
	archive := filepath.Join(directory, "packed.mcworld")
	output, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(output)
	err = filepath.WalkDir(worldDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(worldDir, path)
		if err != nil {
			return err
		}
		target, err := writer.Create("saved/" + filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		_, err = io.Copy(target, input)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err = output.Close(); err != nil {
		t.Fatal(err)
	}
	check(archive)
}
