package main

import (
	"fmt"

	core "neomega-blocks/geometry"
	jmodel "neomega-blocks/geometry/java_block_model"
	"neomega-blocks/ir"
	"neomega-viewer/dispatcher"
)

func newPipeline(tables *core.Tables[uint32], atlas *jmodel.Atlas) (*dispatcher.Pipeline, error) {
	// 占位只持有石头形状的独立副本，不改变共享几何表的 ID 空间。
	stone, err := tables.ModelIDs(ir.IRBlock("minecraft:stone []").Hash())
	if err != nil || len(stone) == 0 {
		return nil, fmt.Errorf("geometry table requires stone for missing-state marker")
	}
	shape := tables.Shapes[stone[0]]
	shape.Quads = append([]core.Quad(nil), shape.Quads...)
	shape.CullGroup = 0
	for i := range shape.Quads {
		shape.Quads[i].Surface.Unlit = true
		shape.Quads[i].Color = [3]float32{1, 0, 1}
	}
	return dispatcher.NewJavaPipeline(tables, atlas, &shape), nil
}
