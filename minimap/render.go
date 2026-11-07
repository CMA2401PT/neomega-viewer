package minimap

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"

	"celler"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer-core/viewer/topview"
)

const TileSize = 128
const TileChunks = TileSize / 16

type Dimension uint8

const (
	Overworld Dimension = iota
	Nether
	End
)

func NewTerrain(dimension Dimension) (Terrain, error) {
	// 两种存档共用维度高度和颜色表，来源只负责把自己的维度名称映射过来。
	colors, err := topview.Bundled()
	if err != nil {
		return Terrain{}, err
	}
	terrain := Terrain{Colors: colors, MinSection: -4, MaxSection: 19}
	switch dimension {
	case Nether:
		terrain.MinSection, terrain.MaxSection, terrain.Nether = 0, 7, true
	case End:
		terrain.MinSection, terrain.MaxSection = 0, 15
	}
	return terrain, nil
}

type Terrain struct {
	// 高度以 section 为单位；下界先穿过顶部实体层，再寻找洞内地表。
	Colors                 topview.Table
	MinSection, MaxSection int32
	Nether                 bool
}

func (t Terrain) Column(ctx context.Context, reader dispatcher.SectionSource, pos celler.ChunkPos) (topview.Column, error) {
	// 逐层解调色板，所有像素都有地表后立即停止，避免读取地下层。
	var result topview.Column
	var opened [256]bool
	remaining := 256
	for sy := t.MaxSection; sy >= t.MinSection && remaining > 0; sy-- {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		section, err := reader.Section(ctx, dispatcher.SectionPos{X: pos.X, Y: sy, Z: pos.Z})
		if err != nil {
			return result, err
		}
		if section == nil || section.Blocks == nil {
			for i := range opened {
				opened[i] = true
			}
			continue
		}
		blocks := section.Blocks
		colors := make([]color.NRGBA, len(blocks.Palette))
		air := make([]bool, len(blocks.Palette))
		for i, block := range blocks.Palette {
			switch block.ShortName() {
			case "", "air", "cave_air", "void_air":
				air[i] = true
				continue
			case "barrier", "light", "structure_void":
				continue
			}
			rgb, ok := t.Colors.Color(block)
			if !ok {
				rgb = 0x808080
			}
			colors[i] = color.NRGBA{R: uint8(rgb >> 16), G: uint8(rgb >> 8), B: uint8(rgb), A: 255}
		}
		for z := range 16 {
			for x := range 16 {
				pixel := z*16 + x
				if result.Pixel[pixel].A != 0 {
					continue
				}
				for y := 15; y >= 0; y-- {
					slot := blocks.Indices[x<<8|z<<4|y]
					if t.Nether && !opened[pixel] {
						opened[pixel] = air[slot]
						continue
					}
					if colors[slot].A == 0 {
						continue
					}
					result.Pixel[pixel] = colors[slot]
					result.Height[pixel] = sy*16 + int32(y)
					remaining--
					break
				}
			}
		}
	}
	return result, nil
}

func (t Terrain) Tile(ctx context.Context, factory func() dispatcher.SectionSource, pos Position) ([]byte, error) {
	// 每根柱子释放自己的解码结果，整张图只保留颜色和高度。
	pixels := make([]color.NRGBA, TileSize*TileSize)
	heights := make([]int32, len(pixels))
	for z := range TileChunks {
		for x := range TileChunks {
			chunkPos := celler.ChunkPos{X: pos.X*TileChunks + int32(x), Z: pos.Z*TileChunks + int32(z)}
			column, err := t.Column(ctx, factory(), chunkPos)
			if err != nil {
				return nil, err
			}
			for lz := range 16 {
				for lx := range 16 {
					target := (z*16+lz)*TileSize + x*16 + lx
					pixels[target] = column.Pixel[lz*16+lx]
					heights[target] = column.Height[lz*16+lx]
				}
			}
		}
	}
	topview.Shade(pixels, heights, TileSize)
	return encode(pixels)
}

func encode(pixels []color.NRGBA) ([]byte, error) {
	// 两种比例的图块使用相同像素尺寸，页面只改变它覆盖的世界范围。
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
