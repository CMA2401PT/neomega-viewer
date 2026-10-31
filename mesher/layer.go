package mesher

import (
	"fmt"

	blockgeometry "neomega-blocks/geometry"
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
	return m.MeshMultiLayerSubchunks([][]B{blocks}, options)
}

func (m *Mesher[B]) MeshMultiLayerSubchunks(layerBlocks [][]B, options Options) (*Result, error) {
	// 每层均为 4096 格 RSL，层号只决定同格不透明共面的覆盖先后。
	// 固定 16³ 格才能按 (x<<8)|(z<<4)|y 直接寻址邻居。
	const blockCount = meshing.Size * meshing.Size * meshing.Size
	for layer, blocks := range layerBlocks {
		if len(blocks) != blockCount {
			return nil, fmt.Errorf("layer %d: subchunk requires %d block IDs, got %d", layer, blockCount, len(blocks))
		}
	}
	cells, err := prepareNeighborhood(layerBlocks, m.tables)
	if err != nil {
		return nil, err
	}
	mesh, stats := meshing.Build(cells, options.Meshing)
	visibleBlocks, shapes := countGeometry(cells)
	return &Result{Mesh: mesh, Stats: Stats{Stats: stats, Blocks: visibleBlocks, Shapes: shapes}}, nil
}

type localGeometry struct {
	meshing.LayerShape
	// 满高度引用兼作流体标记，普通形状不需要邻居选择。
	fullHeight *geometry.Shape
}

func queryGeometry[B blockgeometry.BlockID](tables *blockgeometry.Tables[B], id B, layer int) ([]localGeometry, error) {
	ids, err := tables.ModelIDs(id)
	if err != nil {
		return nil, err
	}
	parts := make([]localGeometry, 0, len(ids))
	for _, id := range ids {
		shape, err := tables.Shape(id)
		if err != nil {
			return nil, err
		}
		local := localGeometry{LayerShape: meshing.LayerShape{Shape: shape, Layer: layer}}
		if full, fluid := tables.FullHeight[id]; fluid {
			local.fullHeight, err = tables.Shape(full)
			if err != nil {
				return nil, err
			}
		}
		parts = append(parts, local)
	}
	return parts, nil
}

func prepareNeighborhood[B blockgeometry.BlockID](layerBlocks [][]B, tables *blockgeometry.Tables[B]) (*meshing.Cells, error) {
	// 先取得原始几何，再读邻居，避免遍历顺序影响流体选择。
	var locals [meshing.Size * meshing.Size * meshing.Size][]localGeometry
	for layer, blocks := range layerBlocks {
		for i, id := range blocks {
			parts, err := queryGeometry(tables, id, layer)
			if err != nil {
				p := position(i)
				return nil, fmt.Errorf("layer %d, cell (%d,%d,%d), ID %d: %w", layer, p.X, p.Y, p.Z, id, err)
			}
			locals[i] = append(locals[i], parts...)
		}
	}
	cells := &meshing.Cells{}
	for i, parts := range locals {
		for _, local := range parts {
			part := local.LayerShape
			if local.fullHeight != nil && position(i).Y < meshing.Size-1 {
				for _, above := range locals[i+1] {
					if above.fullHeight != nil && above.fullHeight.CullGroup == local.fullHeight.CullGroup {
						// 同格还可能有实体或植物，只替换这一份流体引用。
						part.Shape = local.fullHeight
						break
					}
				}
			}
			cells[i] = append(cells[i], part)
		}
	}
	return cells, nil
}

func countGeometry(cells *meshing.Cells) (int, int) {
	blocks := 0
	used := map[*geometry.Shape]struct{}{}
	for _, cell := range cells {
		visible := false
		for _, part := range cell {
			used[part.Shape] = struct{}{}
			visible = visible || len(part.Quads) != 0
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
