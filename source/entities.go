package source

import (
	"fmt"
	"math"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer-core/viewer/entity_render"
	"neomega-viewer-core/viewer/geometry"
)

func GroupEntities(entities []nbt.Compound) (map[dispatcher.SectionPos]*entity_render.Input, []Issue) {
	// Pos 决定唯一所属区域；几何可越界，但不会重复加入邻居。NBT 始终只读借用。
	grouped := map[dispatcher.SectionPos]*entity_render.Input{}
	var issues []Issue
	for index, entity := range entities {
		id, validID := entity["id"].(string)
		position, validPos := entity["Pos"].([]float64)
		if !validID || !validPos || len(position) != 3 {
			issues = append(issues, Issue{Reason: "invalid_entity", Count: 1, Detail: fmt.Sprintf("Entities[%d]: expected string id and three-double Pos", index)})
			continue
		}
		local := geometry.Vec3{}
		var section [3]int32
		for axis, value := range position {
			section[axis] = int32(math.Floor(value / 16))
			local[axis] = value - float64(section[axis])*16
		}
		pos := dispatcher.SectionPos{X: section[0], Y: section[1], Z: section[2]}
		if grouped[pos] == nil {
			grouped[pos] = &entity_render.Input{}
		}
		grouped[pos].List = append(grouped[pos].List, entity_render.Entity{ID: id, Pos: local, NBT: entity})
	}
	return grouped, issues
}
