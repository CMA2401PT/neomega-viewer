// Package subchunk renders one isolated 16x16x16 plain layer. It has no world
// loader or worker and uses shared static geometry tables.
package subchunk

import (
	"fmt"

	"celler/layer_storage"
	"neomega-viewer/blockmesh"
	"neomega-viewer/geometry"
	"neomega-viewer/meshing"
)

type Options struct {
	Meshing meshing.Options
}

type Stats struct {
	meshing.Stats
	// Blocks counts cells with local geometry before neighbor culling.
	Blocks int `json:"blocks"`
	// 统计实际引用的共享形状，不再表示运行时烘焙次数。
	Shapes int `json:"shapes"`
}

type Result struct {
	Mesh  geometry.Mesh
	Stats Stats
}

type Engine struct {
	Blocks *blockmesh.Resolver
}

func New(blocks *blockmesh.Resolver) *Engine {
	return &Engine{Blocks: blocks}
}

func (engine *Engine) Build(layer *layer_storage.Plain[uint32], options Options) (*Result, error) {
	tables, err := engine.Blocks.Tables()
	if err != nil {
		return nil, err
	}
	cells, err := prepareNeighborhood(layer, tables)
	if err != nil {
		return nil, err
	}
	mesh, stats := meshing.Build(cells, options.Meshing)
	blocks, shapes := countGeometry(cells)
	return &Result{Mesh: mesh, Stats: Stats{Stats: stats, Blocks: blocks, Shapes: shapes}}, nil
}

type localGeometry struct {
	parts meshing.Cell
	// 满高度引用兼作流体标记，普通形状不需要邻居选择。
	fullHeight *geometry.Shape
}

func queryGeometry(tables *blockmesh.Tables, javaHash uint32) (localGeometry, error) {
	ids, err := tables.ModelIDs(javaHash)
	if err != nil {
		return localGeometry{}, err
	}
	local := localGeometry{parts: make(meshing.Cell, 0, len(ids))}
	for _, id := range ids {
		shape, err := tables.Shape(id)
		if err != nil {
			return localGeometry{}, err
		}
		local.parts = append(local.parts, shape)
		if full, fluid := tables.FullHeight[id]; fluid {
			local.fullHeight, err = tables.Shape(full)
			if err != nil {
				return localGeometry{}, err
			}
		}
	}
	return local, nil
}

func prepareNeighborhood(layer *layer_storage.Plain[uint32], tables *blockmesh.Tables) (*meshing.Cells, error) {
	// 先取得原始几何，再读邻居，避免遍历顺序影响流体选择。
	var locals [Volume]localGeometry
	for i, javaHash := range layer.Data {
		local, err := queryGeometry(tables, javaHash)
		if err != nil {
			p := position(i)
			return nil, fmt.Errorf("cell (%d,%d,%d), ID %d: %w", p.X, p.Y, p.Z, javaHash, err)
		}
		locals[i] = local
	}
	cells := &meshing.Cells{}
	for i, local := range locals {
		cells[i] = local.parts
		if local.fullHeight == nil || position(i).Y == Size-1 {
			continue
		}
		above := locals[i+1].fullHeight
		if above != nil && above.CullGroup == local.fullHeight.CullGroup {
			cells[i] = meshing.Cell{local.fullHeight}
		}
	}
	return cells, nil
}

func countGeometry(cells *meshing.Cells) (int, int) {
	blocks := 0
	used := map[*geometry.Shape]struct{}{}
	for _, cell := range cells {
		visible := false
		for _, shape := range cell {
			used[shape] = struct{}{}
			visible = visible || len(shape.Quads) != 0
		}
		if visible {
			blocks++
		}
	}
	return blocks, len(used)
}
