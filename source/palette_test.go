package source

import (
	"fmt"
	"slices"
	"testing"

	"celler/layer_storage"
	"neomega-blocks/ir"
	"neomega-viewer-core/viewer/blockdata"
)

func TestPaletteFromHashes(t *testing.T) {
	state := ir.IRBlock(`minecraft:oak_slab ["type"="bottom","waterlogged"="true"]`)
	blocks := make([]uint32, 4096)
	for i := range blocks {
		blocks[i] = ir.IRBlock("minecraft:air []").Hash()
	}
	blocks[4<<8|6<<4|5] = state.Hash()
	palette, err := PaletteFromHashes(blocks)
	if err != nil || palette.Get(layer_storage.At(4, 5, 6)) != state {
		t.Fatal("complete state not restored", err)
	}
	blocks[0] = ir.IRBlock("example:unknown []").Hash()
	if _, err := PaletteFromHashes(blocks); err == nil {
		t.Fatal("unknown hash accepted")
	}
}

// 用 celler 的写入接口生成压缩字，覆盖所有实际可能的位宽、字间填充和最后不足一字的格子。
func TestBlocksFromPackedPalette(t *testing.T) {
	for level := 0; level <= 12; level++ {
		t.Run(fmt.Sprint(level), func(t *testing.T) {
			palette := packedFixture(level)
			originalData := slices.Clone(palette.Data)
			originalPalette := slices.Clone(palette.Palette)
			blocks := BlocksFromPalette(palette)
			if len(blocks.Indices) != 4096 || !slices.Equal(blocks.Palette, palette.Palette[:palette.LastPaletteIdx+1]) {
				t.Fatal("调色板或输入格数丢失")
			}
			for i := range blocks.Indices {
				pos := layer_storage.At(i>>8, i&15, i>>4&15)
				if blocks.Get(i) != palette.Get(pos) || blocks.Indices[i] != uint32(palette.GetIndex(pos)) {
					t.Fatalf("位宽 %d，第 %d 格不一致", level, i)
				}
			}
			blocks.Set(0, ir.IRBlock("example:modified []"))
			blocks.Palette[0] = "example:replaced []"
			if !slices.Equal(palette.Data, originalData) || !slices.Equal(palette.Palette, originalPalette) {
				t.Fatal("转换结果改写了源存储")
			}
		})
	}
}

func packedFixture(level int) *layer_storage.Palette[ir.IRBlock] {
	palette := &layer_storage.Palette[ir.IRBlock]{}
	palette.Fill(level, ir.IRBlock("minecraft:air []"))
	// 保留空闲调色板容量，验证适配不会把未使用的尾部条目作为有效状态。
	count := 1 << level
	if level > 1 {
		count--
	}
	states := make([]ir.IRBlock, count)
	for i := range states {
		states[i] = ir.IRBlock(fmt.Sprintf("example:block_%d []", i))
	}
	palette.SetPalette(states)
	for i := range 4096 {
		palette.SetIndex(layer_storage.At(i>>8, i&15, i>>4&15), (i*37+i/11)%count)
	}
	// 用非零填充验证解包不会把字尾剩余位当成下一个索引。
	if level > 0 {
		perWord := int(palette.IdxsPerUint32)
		for word := range palette.Data {
			used := min(perWord, 4096-word*perWord)
			palette.Data[word] |= ^uint32(0) << (used * level)
		}
	}
	return palette
}

var convertedBlocks *blockdata.Blocks

// 对比整条适配路径的耗时和分配，两边都生成归调用者所有的同一份展开格式。
func BenchmarkBlocksFromPalette(b *testing.B) {
	for _, level := range []int{0, 1, 4, 8, 12} {
		palette := packedFixture(level)
		for _, method := range []string{"bulk", "GetIndex"} {
			b.Run(fmt.Sprintf("bits=%d/%s", level, method), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if method == "bulk" {
						convertedBlocks = BlocksFromPalette(palette)
					} else {
						blocks := &blockdata.Blocks{Palette: slices.Clone(palette.Palette[:palette.LastPaletteIdx+1]), Indices: make([]uint32, 4096)}
						for i := range blocks.Indices {
							blocks.Indices[i] = uint32(palette.GetIndex(layer_storage.At(i>>8, i&15, i>>4&15)))
						}
						convertedBlocks = blocks
					}
				}
			})
		}
	}
}
