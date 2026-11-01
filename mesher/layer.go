package mesher

import (
	"fmt"

	blockgeometry "neomega-blocks/geometry"
	"neomega-viewer/mesher/geometry"
	"neomega-viewer/mesher/meshing"
)

type Options struct {
	Meshing meshing.Options
	// 缺失 ID 可直接引用调用方的形状，避免向共享表注册临时 ID。
	MissingShape *blockgeometry.Shape
}

type Stats struct {
	meshing.Stats
	// Blocks counts cells with local geometry before neighbor culling.
	Blocks int `json:"blocks"`
	// 统计实际引用的共享形状，不再表示运行时烘焙次数。
	Shapes int `json:"shapes"`
}

type MissingCell struct {
	// 仅标记中心 section，邻居占位只用于覆盖查询。
	Pos   [3]uint8
	Layer int
}

type Result struct {
	Mesh    geometry.Mesh
	Missing []MissingCell
	Stats   Stats
}

func (m *Mesher[B]) MeshSubchunk(blocks []B, options Options) (*Result, error) {
	return m.MeshMultiLayerSubchunks([][]B{blocks}, options)
}

func (m *Mesher[B]) MeshMultiLayerSubchunks(layerBlocks [][]B, options Options) (*Result, error) {
	return m.MeshWithNeighbours(layerBlocks, [6][][]B{}, options)
}

// MeshWithNeighbours accepts adjacent layers in west/east/down/up/north/south
// order. Missing neighbours are exposed space. Inputs remain owned by the caller.
func (m *Mesher[B]) MeshWithNeighbours(layerBlocks [][]B, neighbours [6][][]B, options Options) (*Result, error) {
	// 每层均为 4096 格 RSL，层号只决定同格不透明共面的覆盖先后。
	// 固定 16³ 格才能按 (x<<8)|(z<<4)|y 直接寻址邻居。
	const blockCount = meshing.Size * meshing.Size * meshing.Size
	for layer, blocks := range layerBlocks {
		if len(blocks) != blockCount {
			return nil, fmt.Errorf("layer %d: subchunk requires %d block IDs, got %d", layer, blockCount, len(blocks))
		}
	}
	boundaries, err := m.boundaries(layerBlocks, neighbours, options.MissingShape)
	if err != nil {
		return nil, err
	}
	cells, missing, err := prepareNeighborhood(layerBlocks, m.tables, boundaries, options.MissingShape)
	if err != nil {
		return nil, err
	}
	options.Meshing.Neighbours = boundaries
	mesh, stats := meshing.Build(cells, options.Meshing)
	visibleBlocks, shapes := countGeometry(cells)
	return &Result{
		Mesh:    mesh,
		Missing: missing,
		Stats: Stats{
			Stats:  stats,
			Blocks: visibleBlocks,
			Shapes: shapes,
		},
	}, nil
}

type localGeometry struct {
	meshing.LayerShape
	// 满高度引用兼作流体标记，普通形状不需要邻居选择。
	fullHeight *geometry.Shape
}

func queryGeometry[B blockgeometry.BlockID](
	tables *blockgeometry.Tables[B],
	id B,
	layer int,
	missing *blockgeometry.Shape,
) ([]localGeometry, bool, error) {
	// 空模型列表是有意无几何；只有键不存在才使用占位。
	ids, found := tables.Index[id]
	if !found {
		if missing != nil {
			return []localGeometry{{LayerShape: meshing.LayerShape{Shape: missing, Layer: layer}}}, true, nil
		}
		_, err := tables.ModelIDs(id)
		return nil, true, err
	}
	parts := make([]localGeometry, 0, len(ids))
	for _, id := range ids {
		shape, err := tables.Shape(id)
		if err != nil {
			return nil, false, err
		}
		local := localGeometry{LayerShape: meshing.LayerShape{Shape: shape, Layer: layer}}
		if full, fluid := tables.FullHeight[id]; fluid {
			local.fullHeight, err = tables.Shape(full)
			if err != nil {
				return nil, false, err
			}
		}
		parts = append(parts, local)
	}
	return parts, false, nil
}

func prepareNeighborhood[B blockgeometry.BlockID](
	layerBlocks [][]B,
	tables *blockgeometry.Tables[B],
	boundaries *meshing.Boundaries,
	missingShape *blockgeometry.Shape,
) (*meshing.Cells, []MissingCell, error) {
	// 先取得原始几何，再读邻居，避免遍历顺序影响流体选择。
	var locals [meshing.Size * meshing.Size * meshing.Size][]localGeometry
	var missing []MissingCell
	for layer, blocks := range layerBlocks {
		for i, id := range blocks {
			parts, absent, err := queryGeometry(tables, id, layer, missingShape)
			if err != nil {
				p := position(i)
				return nil, nil, fmt.Errorf("layer %d, cell (%d,%d,%d), ID %d: %w", layer, p.X, p.Y, p.Z, id, err)
			}
			if absent {
				p := position(i)
				missing = append(missing, MissingCell{Pos: [3]uint8{p.X, p.Y, p.Z}, Layer: layer})
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
			if local.fullHeight != nil && position(i).Y == meshing.Size-1 && boundaries != nil {
				p := position(i)
				for _, above := range boundaries[3][meshing.BoundaryIndex(3, [3]int{int(p.X), int(p.Y), int(p.Z)})] {
					if above.CullGroup == local.fullHeight.CullGroup {
						part.Shape = local.fullHeight
						break
					}
				}
			}
			cells[i] = append(cells[i], part)
		}
	}
	return cells, missing, nil
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

func (m *Mesher[B]) boundaries(
	center [][]B,
	neighbours [6][][]B,
	missing *blockgeometry.Shape,
) (*meshing.Boundaries, error) {
	result := &meshing.Boundaries{}
	for side, layers := range neighbours {
		for layer, values := range layers {
			if len(values) != 4096 {
				return nil, fmt.Errorf("neighbour %d layer %d: expected 4096 blocks", side, layer)
			}
			for a := 0; a < 16; a++ {
				for b := 0; b < 16; b++ {
					edge := 15
					if side%2 == 1 {
						edge = 0
					}
					p := [3]int{edge, b, a}
					if side/2 == 1 {
						p = [3]int{a, edge, b}
					} else if side/2 == 2 {
						p = [3]int{a, b, edge}
					}
					index := p[0]<<8 | p[2]<<4 | p[1]
					parts, _, err := queryGeometry(m.tables, values[index], layer, missing)
					if err != nil {
						return nil, err
					}
					for _, part := range parts {
						shape := part.LayerShape
						aboveLayers, aboveIndex := layers, index+1
						if p[1] == 15 {
							aboveLayers = nil
							if side == 2 {
								// The lower neighbour's top fluid touches this section's bottom.
								aboveLayers, aboveIndex = center, index-15
							}
						}
						if part.fullHeight != nil {
							for aboveLayer, aboveValues := range aboveLayers {
								above, _, err := queryGeometry(m.tables, aboveValues[aboveIndex], aboveLayer, missing)
								if err != nil {
									return nil, err
								}
								for _, candidate := range above {
									if candidate.fullHeight != nil && candidate.fullHeight.CullGroup == part.fullHeight.CullGroup {
										shape.Shape = part.fullHeight
									}
								}
							}
						}
						slot := meshing.BoundaryIndex(side, p)
						result[side][slot] = append(result[side][slot], shape)
					}
				}
			}
		}
	}
	return result, nil
}
