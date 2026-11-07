package source

import (
	"fmt"
	"slices"

	"celler/layer_storage"
	"neomega-blocks/ir"
	java "neomega-viewer-core/viewer/javastates"
	"neomega-viewer-core/viewer/blockdata"
	"neomega-viewer-core/viewer/splitlayer"
)

// BlocksFromPalette 将世界的固定 16³ 存储展开为渲染输入，保持 X-Z-Y 顺序。
// 按压缩字顺序解包，避免逐格坐标转换和除法；单值 Palette 直接使用全零索引。
// 调色板和索引均归返回值所有，后续构造输入不会改动存档缓存。
func BlocksFromPalette(palette *layer_storage.Palette[ir.IRBlock]) *blockdata.Blocks {
	blocks := &blockdata.Blocks{
		Palette: slices.Clone(palette.Palette[:palette.LastPaletteIdx+1]),
		Indices: make([]uint32, 4096),
	}
	// celler 的索引不跨 uint32 边界，字尾不足一格的位是填充。
	remaining := blocks.Indices
	for _, packed := range palette.Data {
		count := min(int(palette.IdxsPerUint32), len(remaining))
		for i := range remaining[:count] {
			remaining[i] = packed & palette.Mask
			packed >>= palette.Level
		}
		remaining = remaining[count:]
	}
	return blocks
}

// PaletteFromHashes 为固定 16³ 的演示输入恢复完整 Java 状态。
func PaletteFromHashes(blocks []splitlayer.JavaBlockHash) (*layer_storage.Palette[ir.IRBlock], error) {
	// hash 输入只能恢复预置表中已知的状态；原生存档直接传 IR 调色板。
	palette := layer_storage.MakePaletteStorage(ir.IRBlock("minecraft:air []"))
	states := map[uint32]ir.IRBlock{}
	for i, hash := range blocks {
		state, found := states[hash]
		if !found {
			state, found = java.HashToBlock(hash)
			if !found {
				return nil, fmt.Errorf("unknown Java block hash %d", hash)
			}
			states[hash] = state
		}
		palette.Set(layer_storage.At(i>>8, i&15, i>>4&15), state)
	}
	return palette, nil
}
