package source

import (
	"testing"

	nbt "neomega-nbt/presets/v1"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer-core/viewer/geometry"
)

func TestGroupEntitiesOwnsNegativeFloatPositions(t *testing.T) {
	// 浮点位置向下取整决定唯一所属区域，边界负坐标不能向零截断。
	entity := nbt.Compound{"id": "minecraft:item_frame", "Pos": []float64{-16.25, -.5, 32}}
	grouped, issues := GroupEntities([]nbt.Compound{entity, {"id": "bad", "Pos": []float64{1, 2}}})
	input := grouped[dispatcher.SectionPos{X: -2, Y: -1, Z: 2}]
	if len(grouped) != 1 || input == nil || len(input.List) != 1 || input.List[0].Pos != (geometry.Vec3{15.75, 15.5, 0}) || input.Query != nil || len(issues) != 1 {
		t.Fatalf("grouped=%+v issues=%+v", grouped, issues)
	}
	if _, exists := entity["Facing"]; exists {
		t.Fatal("source changed entity NBT")
	}
}
