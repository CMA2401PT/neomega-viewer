package javaworld

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"celler"
	"celler/java"
	"celler/java/region"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer-core/viewer/cube"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer-core/viewer/javapreset"
	preset "neomega-viewer-core/viewer/resources"
	"neomega-viewer/source"
)

func TestEntitiesRecordWithoutTerrain(t *testing.T) {
	// 实体 region 独立存在时仍返回无方块的 section，反向缺失也应独立表达。
	world, err := java.OpenDirectory(t.TempDir(), java.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer world.Close()
	entities := []nbt.Compound{{"id": "minecraft:item_frame", "Pos": []float64{1.5, -60, 2.5}, "Facing": int8(2)}}
	data, err := nbt.EncodeDocument(nbt.Document{Value: nbt.Compound{"Entities": entities}}, nbt.EncodingBigEndian)
	if err != nil {
		t.Fatal(err)
	}
	if err := world.WriteRaw(java.Overworld, java.Entities, celler.ChunkPos{}, region.Record{Compression: region.Uncompressed, Data: data}); err != nil {
		t.Fatal(err)
	}
	chunk := java.NewChunk(celler.ChunkPos{X: 1}, 4189)
	if err := world.WriteChunk(java.Overworld, chunk); err != nil {
		t.Fatal(err)
	}
	if err := world.Commit(); err != nil {
		t.Fatal(err)
	}
	src := New(world, java.Overworld)
	defer src.Close()
	records, err := src.read(context.Background(), celler.ChunkPos{})
	if err != nil || records[0] != nil || records[1] == nil {
		t.Fatalf("entity-only records: %v, %v", records, err)
	}
	section, err := src.Reader().Section(context.Background(), dispatcher.SectionPos{Y: -4})
	if err != nil || section == nil || section.Blocks != nil || section.Entities == nil || len(section.Entities.List) != 1 {
		t.Fatalf("entity-only section: %+v, %v", section, err)
	}
	records, err = src.read(context.Background(), celler.ChunkPos{X: 1})
	if err != nil || records[0] == nil || records[1] != nil {
		t.Fatalf("terrain-only records: %v, %v", records, err)
	}
}

func TestFiveFrameWorld(t *testing.T) {
	// 指定存档是验收输入；只检查主世界原点柱子中的五框，不遍历其他实体场景。
	path := os.Getenv("NEOMEGA_FRAME_WORLD")
	if path == "" {
		t.Skip("set NEOMEGA_FRAME_WORLD to the five-frame overworld directory")
	}
	world, err := java.OpenDirectory(filepath.Clean(path), java.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer world.Close()
	src := New(world, java.Overworld)
	defer src.Close()
	reader := src.Reader()
	section, err := reader.Section(context.Background(), dispatcher.SectionPos{Y: -4})
	if err != nil || section == nil || section.Entities == nil {
		t.Fatalf("five-frame input: %+v, %v", section, err)
	}
	if len(section.Entities.List) != 5 {
		t.Fatalf("frame count: %d", len(section.Entities.List))
	}
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := javapreset.NewPipeline(cube.New(16), resources.Tables, resources.Items, nil)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	glowing, empty, wantQuads := 0, 0, 0
	for _, entity := range section.Entities.List {
		if entity.ID == "minecraft:glow_item_frame" {
			glowing++
		} else if entity.ID != "minecraft:item_frame" {
			t.Fatalf("unexpected entity: %s", entity.ID)
		}
		modelID := "minecraft:block/item_frame"
		if entity.ID == "minecraft:glow_item_frame" {
			modelID = "minecraft:block/glow_item_frame"
		}
		wantQuads += len(resources.Items.Models[modelID].Quads)
		if item, ok := entity.NBT["Item"].(map[string]any); ok {
			id := item["id"].(string)
			counts[id]++
			wantQuads += len(resources.Items.Items[id].Quads)
		} else {
			empty++
		}
	}
	if glowing != 1 || empty != 1 || counts["minecraft:diamond_sword"] != 2 || counts["minecraft:sea_lantern"] != 2 {
		t.Fatalf("glow=%d empty=%d contents=%v", glowing, empty, counts)
	}
	// 只给同一批真实实体，移除方块后检查每个框和物品都贡献静态几何。
	result, err := pipeline.Build(dispatcher.Section{Entities: section.Entities}, dispatcher.Neighbours{}, dispatcher.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Frame.Mesh.Quads) != wantQuads || len(result.EntityDiagnostics) != 0 || len(result.Diagnostics) != 0 || len(result.Frame.Actors) != 0 {
		t.Fatalf("entity output quads=%d want=%d issues=%+v", len(result.Frame.Mesh.Quads), wantQuads, result.EntityDiagnostics)
	}
	// 同一输入也经过完整地形流程，确保实体没有因来源或分层而丢失。
	full, err := pipeline.Build(*section, dispatcher.Neighbours{}, dispatcher.Options{})
	if err != nil || len(full.Frame.Mesh.Quads) < wantQuads || len(full.EntityDiagnostics) != 0 {
		t.Fatalf("world output: mesh=%d issues=%+v err=%v", len(full.Frame.Mesh.Quads), full.EntityDiagnostics, err)
	}
	if issues := reader.(source.ColumnDiagnostics).ColumnDiagnostics(celler.ChunkPos{}); len(issues) != 0 {
		t.Fatal(issues)
	}
}
