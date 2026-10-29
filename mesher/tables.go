package mesher

import (
	"fmt"

	"neomega-viewer/geometry"
)

type ModelID uint32

type Shape = geometry.Shape

type Tables[B BlockID] struct {
	// 表和查询返回的切片共享且只读，供多个渲染入口复用。
	Index  map[B][]ModelID
	Shapes []Shape
	// 仅流体几何有此映射，上方同组几何出现时改用满高度几何。
	FullHeight map[ModelID]ModelID
}

func (tables *Tables[B]) ModelIDs(id B) ([]ModelID, error) {
	ids, ok := tables.Index[id]
	if !ok {
		return nil, fmt.Errorf("no static geometry for block ID %d", id)
	}
	return ids, nil
}

func (tables *Tables[B]) Shape(id ModelID) (*Shape, error) {
	if uint64(id) >= uint64(len(tables.Shapes)) {
		return nil, fmt.Errorf("unknown model ID %d", id)
	}
	return &tables.Shapes[id], nil
}
