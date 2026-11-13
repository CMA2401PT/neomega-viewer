package minimap

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math/bits"

	"neomega-viewer-core/viewer/brickmap"
	"neomega-viewer-core/viewer/dispatcher"
)

const TileSize = 128
const RegionSize = 128
const RegionChunks = RegionSize / 16

type Dimension uint8

const (
	Overworld Dimension = iota
	Nether
	End
)

func NewTerrain(dimension Dimension) (Terrain, error) {
	// 材质资源在各区域之间共享，材质编号则由每个区域独立分配。
	materials, err := brickmap.OpenMaterialSource()
	if err != nil {
		return Terrain{}, err
	}
	terrain := Terrain{Materials: materials, MinSection: -4, MaxSection: 19}
	switch dimension {
	case Nether:
		terrain.MinSection, terrain.MaxSection = 0, 7
	case End:
		terrain.MinSection, terrain.MaxSection = 0, 15
	}
	return terrain, nil
}

type Terrain struct {
	// 来源高度以 section 为单位，目录高度补齐为空气以满足 mip 层级布局。
	Materials              *brickmap.MaterialSource
	MinSection, MaxSection int32
}

func (t Terrain) Bounds() [2]int {
	// 页面无需等待全世界索引，就能按维度高度创建窗口目录。
	return [2]int{int(t.MinSection) * 16, 1 << bits.Len(uint((t.MaxSection-t.MinSection+1)*16-1))}
}

func (t Terrain) Region(ctx context.Context, factory func() dispatcher.SectionSource, pos Position) (*brickmap.Volume, error) {
	// 每根柱子使用独立 reader，避免解码器把整片区域的原始区块长期留在内存。
	bounds := t.Bounds()
	palette := brickmap.NewPalette(t.Materials)
	builder := brickmap.NewBuilder(
		[3]int{int(pos.X) * RegionSize, bounds[0], int(pos.Z) * RegionSize},
		[3]int{RegionSize, bounds[1], RegionSize},
		nil,
	)
	var cells [4096]uint16
	for z := range RegionChunks {
		for x := range RegionChunks {
			reader := factory()
			for y := t.MinSection; y <= t.MaxSection; y++ {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				position := dispatcher.SectionPos{X: pos.X*RegionChunks + int32(x), Y: y, Z: pos.Z*RegionChunks + int32(z)}
				section, err := reader.Section(ctx, position)
				if err != nil {
					return nil, err
				}
				if section == nil || section.Blocks == nil {
					continue
				}
				ids := make([]uint16, len(section.Blocks.Palette))
				for i, block := range section.Blocks.Palette {
					ids[i] = palette.ID(block)
				}
				for i, id := range section.Blocks.Indices {
					cells[i] = ids[id]
				}
				builder.AddSection([3]int{int(position.X) * 16, int(y) * 16, int(position.Z) * 16}, cells[:])
			}
		}
	}
	volume := builder.Finish()
	volume.Palette = palette.Materials()
	return volume, nil
}

func encode(pixels []color.NRGBA) ([]byte, error) {
	// 占用图每个像素对应一个区块，PNG 中不存在的区块保持透明。
	img := image.NewNRGBA(image.Rect(0, 0, TileSize, TileSize))
	for i, pixel := range pixels {
		img.Pix[i*4] = pixel.R
		img.Pix[i*4+1] = pixel.G
		img.Pix[i*4+2] = pixel.B
		img.Pix[i*4+3] = pixel.A
	}
	var output bytes.Buffer
	err := png.Encode(&output, img)
	return output.Bytes(), err
}
