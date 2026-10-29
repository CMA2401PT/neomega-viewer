package mesher

import (
	"fmt"

	"neomega-viewer/mesher/geometry"
	"neomega-viewer/mesher/meshing"
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

func (m *Mesher[B]) MeshSubchunk(blocks []B, options Options) (*Result, error) {
	// 固定 16³ 格才能按 (x<<8)|(z<<4)|y 直接寻址邻居。
	const blockCount = meshing.Size * meshing.Size * meshing.Size
	if len(blocks) != blockCount {
		return nil, fmt.Errorf("subchunk requires %d block IDs, got %d", blockCount, len(blocks))
	}
	cells, err := prepareNeighborhood(blocks, m.tables)
	if err != nil {
		return nil, err
	}
	mesh, stats := meshing.Build(cells, options.Meshing)
	visibleBlocks, shapes := countGeometry(cells)
	return &Result{Mesh: mesh, Stats: Stats{Stats: stats, Blocks: visibleBlocks, Shapes: shapes}}, nil
}

type localGeometry struct {
	parts meshing.Cell
	// 满高度引用兼作流体标记，普通形状不需要邻居选择。
	fullHeight *geometry.Shape
}

func queryGeometry[B BlockID](tables *Tables[B], id B) (localGeometry, error) {
	ids, err := tables.ModelIDs(id)
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

func prepareNeighborhood[B BlockID](blocks []B, tables *Tables[B]) (*meshing.Cells, error) {
	// 先取得原始几何，再读邻居，避免遍历顺序影响流体选择。
	var locals [meshing.Size * meshing.Size * meshing.Size]localGeometry
	for i, id := range blocks {
		local, err := queryGeometry(tables, id)
		if err != nil {
			p := position(i)
			return nil, fmt.Errorf("cell (%d,%d,%d), ID %d: %w", p.X, p.Y, p.Z, id, err)
		}
		locals[i] = local
	}
	cells := &meshing.Cells{}
	for i, local := range locals {
		cells[i] = local.parts
		if local.fullHeight == nil || position(i).Y == meshing.Size-1 {
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

type blockPosition struct {
	X, Y, Z uint8
}

func position(index int) blockPosition {
	return blockPosition{X: uint8(index >> 8), Y: uint8(index & 15), Z: uint8((index >> 4) & 15)}
}
