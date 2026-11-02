package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	jmodel "neomega-blocks/geometry/java_block_model"
	preset "neomega-blocks/presets/java/v1/geometry"
	"neomega-viewer/dispatcher"
	"neomega-viewer/viewassets"
)

// 验证正负坐标重建一致，且真实资源能生成纹样陶罐和动态书本消息。
func TestGeneratedWorld(t *testing.T) {
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	atlasJSON, err := fs.ReadFile(viewassets.Resources(), "blocksAtlases.json")
	if err != nil {
		t.Fatal(err)
	}
	var atlas jmodel.Atlas
	if err := json.Unmarshal(atlasJSON, &atlas); err != nil {
		t.Fatal(err)
	}
	pipeline := dispatcher.NewJavaPipeline(resources.Tables, &atlas, nil)
	world := generatedWorld{}
	for _, pos := range []dispatcher.SectionPos{{}, {X: -1, Z: -1}, {X: 12345, Z: -789}} {
		first, err := world.Section(t.Context(), pos)
		if err != nil {
			t.Fatal(err)
		}
		again, err := world.Section(t.Context(), pos)
		if err != nil || !reflect.DeepEqual(first, again) {
			t.Fatalf("坐标 %v 重建结果不同：%v", pos, err)
		}
		results, err := pipeline.BuildSections(
			t.Context(),
			world,
			[]dispatcher.SectionPos{pos},
			dispatcher.Options{},
		)
		if err != nil {
			t.Fatal(err)
		}
		result := results[0].Result
		if len(result.Diagnostics) != 0 {
			t.Fatalf("坐标 %v 存在渲染诊断：%v", pos, result.Diagnostics)
		}
		frame := result.Frame
		if len(frame.Actors) != 1 || len(frame.Effects) != 1 || frame.Effects[0].Effect != "enchanting_table_book" {
			t.Fatalf("坐标 %v 缺少陶罐或书本", pos)
		}
		patterns := 0
		for _, quad := range frame.Actors[0].Mesh.Quads {
			if strings.HasSuffix(quad.Surface.Texture, "_pottery_pattern") {
				patterns++
			}
		}
		if patterns != 4 {
			t.Fatalf("陶罐应有四面纹样，实际 %d", patterns)
		}
		// 邻居同样填满底层，接缝处不应残留从 Y=0 开始的侧面。
		for _, quad := range frame.Mesh.Quads {
			if quad.Normal[1] != 0 {
				continue
			}
			for _, point := range quad.Positions {
				if point[1] == 0 && (point[0] == 0 || point[0] == 16 || point[2] == 0 || point[2] == 16) {
					t.Fatalf("坐标 %v 接缝有未剔除的底层侧面", pos)
				}
			}
		}
	}
	section, err := world.Section(context.Background(), dispatcher.SectionPos{Y: 1})
	if err != nil || section != nil {
		t.Fatalf("地面以上应为空气：%v", err)
	}
}
