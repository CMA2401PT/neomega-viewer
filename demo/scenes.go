// Package demo builds small, inspectable palette layers for the browser. These
// scene recipes are not part of the block parser or subchunk rendering library.
package demo

import (
	"fmt"

	"celler"
	"celler/layer_storage"
	"neomega-blocks/ir"
	blocks "neomega-blocks/presets/block-converts/v1"
)

type Scene struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

var scenes = []Scene{
	{"mixed", "形状与透明度", "石头、玻璃、半砖、栅栏、草木与特殊模型放在同一个子区块中。"},
	{"solid", "填满石头", "4096 个石头方块。观察面剔除和网格合并各自减少了什么。"},
	{"glass", "玻璃与内部物体", "相同玻璃去掉内面，不同颜色保留界面；透过玻璃观察内部形状。"},
	{"slabs", "半砖交界", "半砖与完整方块相邻，只剔除实际覆盖的部分。"},
	{"foliage", "树叶与交叉平面", "透过镂空贴图观察后面的石头，并检查草和火焰的平面外形。"},
	{"checker", "交错材质", "内部面仍会剔除，纹理不同的外表面保持分开。"},
	{"empty", "空气", "默认填充值也是数据；空气返回空网格。"},
}

func List() []Scene { return append([]Scene(nil), scenes...) }

func Build(name string) (*layer_storage.Palette[uint32], error) {
	valid := false
	for _, scene := range scenes {
		valid = valid || scene.ID == name
	}
	if !valid {
		return nil, fmt.Errorf("unknown demo %q", name)
	}
	ids := map[string]uint32{}
	blockID := func(java string) (uint32, error) {
		if id, found := ids[java]; found {
			return id, nil
		}
		block, ok := blocks.ConvertFromJava(ir.IRBlock(java))
		if !ok {
			return 0, fmt.Errorf("demo state cannot convert: %s", java)
		}
		ids[java] = block.Hash()
		return block.Hash(), nil
	}
	air, err := blockID("minecraft:air []")
	if err != nil {
		return nil, err
	}
	layer := layer_storage.MakePaletteStorage(air)
	var buildError error
	put := func(x, y, z int, java string) {
		if buildError != nil {
			return
		}
		id, err := blockID(java)
		if err != nil {
			buildError = err
			return
		}
		layer.Set(celler.ToSubChunkBlockPos(x, y, z), id)
	}
	box := func(x0, y0, z0, x1, y1, z1 int, java string) {
		for x := x0; x < x1; x++ {
			for z := z0; z < z1; z++ {
				for y := y0; y < y1; y++ {
					put(x, y, z, java)
				}
			}
		}
	}
	switch name {
	case "empty":
	case "solid":
		stone, err := blockID("minecraft:stone []")
		if err != nil {
			return nil, err
		}
		layer.InitWithDefault(stone)
	case "checker":
		for x := 0; x < 16; x++ {
			for y := 0; y < 16; y++ {
				for z := 0; z < 16; z++ {
					java := "minecraft:stone []"
					if (x+y+z)%2 == 1 {
						java = "minecraft:dirt []"
					}
					put(x, y, z, java)
				}
			}
		}
	case "slabs":
		box(2, 0, 2, 14, 1, 14, "minecraft:stone []")
		box(3, 1, 4, 7, 2, 11, "minecraft:stone []")
		box(7, 1, 4, 11, 2, 7, "minecraft:oak_slab [type=bottom]")
		box(7, 1, 8, 11, 2, 11, "minecraft:oak_slab [type=top]")
		box(4, 2, 4, 7, 3, 7, "minecraft:stone_slab [type=bottom]")
	case "glass":
		box(2, 0, 2, 14, 1, 14, "minecraft:stone []")
		box(4, 1, 4, 12, 8, 12, "minecraft:glass []")
		box(5, 1, 5, 11, 7, 11, "minecraft:air []")
		box(7, 1, 7, 9, 5, 9, "minecraft:stone []")
		box(11, 1, 5, 12, 7, 8, "minecraft:red_stained_glass []")
		box(11, 1, 8, 12, 7, 11, "minecraft:blue_stained_glass []")
	case "foliage":
		box(2, 0, 2, 14, 1, 14, "minecraft:stone []")
		box(3, 1, 4, 13, 4, 7, "minecraft:oak_leaves []")
		box(4, 1, 7, 12, 3, 9, "minecraft:stone []")
		for x := 3; x < 13; x += 2 {
			put(x, 1, 11, "minecraft:short_grass []")
		}
		put(12, 1, 11, "minecraft:fire []")
	case "mixed":
		box(1, 0, 1, 15, 1, 15, "minecraft:stone []")
		box(2, 1, 3, 3, 5, 12, "minecraft:glass []")
		box(3, 1, 3, 6, 2, 8, "minecraft:oak_slab [type=bottom]")
		box(3, 1, 9, 6, 2, 12, "minecraft:oak_slab [type=top]")
		for x := 7; x < 14; x++ {
			put(x, 1, 3, "minecraft:oak_fence []")
			put(x, 1, 12, "minecraft:oak_fence []")
		}
		for z := 4; z < 12; z++ {
			put(13, 1, z, "minecraft:oak_fence []")
		}
		box(10, 1, 5, 12, 4, 8, "minecraft:oak_leaves []")
		put(7, 1, 6, "minecraft:short_grass []")
		put(8, 1, 7, "minecraft:short_grass []")
		put(8, 1, 10, "minecraft:candle [candles=4]")
		put(4, 1, 13, "minecraft:chest [facing=south]")
		put(7, 1, 9, "minecraft:red_bed [part=foot,facing=north]")
		put(7, 1, 8, "minecraft:red_bed [part=head,facing=north]")
	}
	return layer, buildError
}
