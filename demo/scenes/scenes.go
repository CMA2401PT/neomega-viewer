// Package demo builds small, inspectable palette layers for the browser. These
// scene recipes are not part of the block parser or subchunk rendering library.
package demo

import (
	"fmt"
	"strings"

	"celler"
	"celler/layer_storage"
	"neomega-blocks/ir"
	converts "neomega-blocks/presets/block-converts/v1"
	java "neomega-blocks/presets/java/v1"
)

type Scene struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

var scenes = []Scene{
	{"mixed", "形状与透明度", "石头、玻璃、半砖、栅栏、草木与特殊模型放在同一个子区块中。"},
	{"fence_dirty", "脏栅栏 · 未修复输入", "默认状态的栅栏排成直线和 L 形，端点挨着石头；开启修复可连接栏杆，三根孤立栅栏保持不连。"},
	{"special_java", "特殊方块 · Java 原版", "旗帜、附魔台、两格植物、含雪与含水方块、海晶灯、床、门和花盆植物，使用原始 Java 状态。"},
	{"special_nemc", "特殊方块 · NEMC 往返", "与 Java 原版使用相同布局，方块状态经过 Java→NEMC→Java 往返，用于对照转换差异。"},
	{"solid", "填满石头", "4096 个石头方块。观察面剔除和网格合并各自减少了什么。"},
	{"glass", "玻璃与内部物体", "相同玻璃去掉内面，不同颜色保留界面；透过玻璃观察内部形状。"},
	{"fluids", "流体与纹理动画", "石池中的水与岩浆展示纹理动画；中央叠放同类流体，检查下层的满高度衔接。"},
	{"waterlogged", "含水方块", "观察含水模型与水的共面裁切、植物和树叶的重叠，以及水池与叠水的跨层衔接。"},
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
	javaBlock := func(recipe string) ir.IRBlock {
		block := ir.IRBlock(recipe).Tidy()
		// 配方可省略默认属性，渲染和 fixer 需要完整状态的 hash。
		properties, _ := java.BlockProperties(block.NameWithNameSpace())
		defaults := make([]string, 0, len(properties))
		for name, property := range properties {
			defaults = append(defaults, fmt.Sprintf("%q=%s", name, property.Default))
		}
		states := ir.IRStates("[" + strings.Join(defaults, ",") + "]").Tidy()
		for name, value := range block.States().KeyValues {
			states = states.Set(name, value)
		}
		return ir.IRBlock(block.NameWithNameSpace() + " " + string(states)).Tidy()
	}
	ids := map[string]uint32{}
	blockID := func(recipe string) uint32 {
		if id, found := ids[recipe]; found {
			return id
		}
		id := javaBlock(recipe).Hash()
		ids[recipe] = id
		return id
	}
	blockIDRoundtrip := func(recipe string) uint32 {
		// 两种来源都先补齐 Java 默认属性，往返失败时保留同一输入状态。
		input := javaBlock(recipe)
		bedrock, ok := converts.ConvertFromJava(input)
		if !ok {
			return input.Hash()
		}
		output, ok := converts.ConvertToJava(bedrock)
		if !ok {
			return input.Hash()
		}
		return output.Tidy().Hash()
	}
	air := blockID("minecraft:air []")
	layer := layer_storage.MakePaletteStorage(air)
	toHash := blockID
	put := func(x, y, z int, java string) {
		layer.Set(celler.ToSubChunkBlockPos(x, y, z), toHash(java))
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
	specialLayout := func(hashRecipe func(string) uint32) {
		// 共用坐标与摆放入口，只替换配方生成 hash 的策略，便于逐格对照。
		toHash = hashRecipe
		box(1, 0, 1, 15, 1, 15, "minecraft:stone []")
		put(3, 1, 3, `minecraft:white_banner ["rotation"="0"]`)
		put(7, 1, 3, "minecraft:enchanting_table []")
		put(11, 1, 3, `minecraft:sunflower ["half"="lower"]`)
		put(11, 2, 3, `minecraft:sunflower ["half"="upper"]`)
		put(3, 1, 7, `minecraft:grass_block ["snowy"="true"]`)
		put(7, 1, 7, `minecraft:oak_slab ["type"="bottom","waterlogged"="true"]`)
		put(11, 1, 7, "minecraft:sea_lantern []")
		// north 朝向负 Z，床头在床尾北侧；门与植物均保留上下两格。
		put(3, 1, 11, `minecraft:red_bed ["part"="foot","facing"="north","occupied"="false"]`)
		put(3, 1, 10, `minecraft:red_bed ["part"="head","facing"="north","occupied"="false"]`)
		put(7, 1, 11, `minecraft:oak_door ["half"="lower","facing"="north","hinge"="left","open"="false","powered"="false"]`)
		put(7, 2, 11, `minecraft:oak_door ["half"="upper","facing"="north","hinge"="left","open"="false","powered"="false"]`)
		put(11, 1, 11, "minecraft:potted_cactus []")
	}
	switch name {
	case "empty":
	case "special_java":
		specialLayout(blockID)
	case "special_nemc":
		specialLayout(blockIDRoundtrip)
	case "solid":
		stone := blockID("minecraft:stone []")
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
		box(7, 1, 4, 11, 2, 7, `minecraft:oak_slab ["type"="bottom"]`)
		box(7, 1, 8, 11, 2, 11, `minecraft:oak_slab ["type"="top"]`)
		box(4, 2, 4, 7, 3, 7, `minecraft:stone_slab ["type"="bottom"]`)
	case "glass":
		box(2, 0, 2, 14, 1, 14, "minecraft:stone []")
		box(4, 1, 4, 12, 8, 12, "minecraft:glass []")
		box(5, 1, 5, 11, 7, 11, "minecraft:air []")
		box(7, 1, 7, 9, 5, 9, "minecraft:stone []")
		box(11, 1, 5, 12, 7, 8, "minecraft:red_stained_glass []")
		box(11, 1, 8, 12, 7, 11, "minecraft:blue_stained_glass []")
	case "fluids":
		// 石底和一格高的池沿，中间留石墙分隔。
		box(1, 0, 3, 15, 2, 12, "minecraft:stone []")
		box(2, 1, 4, 7, 2, 11, `minecraft:water ["level"="0"]`)
		box(9, 1, 4, 14, 2, 11, `minecraft:lava ["level"="0"]`)
		// 中央再叠一层，让下方同类流体达到满高度。
		box(3, 2, 6, 6, 3, 9, `minecraft:water ["level"="0"]`)
		box(10, 2, 6, 13, 3, 9, `minecraft:lava ["level"="0"]`)
	case "waterlogged":
		box(1, 0, 1, 15, 1, 15, "minecraft:stone []")
		put(3, 1, 3, `minecraft:oak_slab ["type"="bottom","waterlogged"="true"]`)
		put(7, 1, 3, `minecraft:oak_stairs ["facing"="south","half"="bottom","shape"="straight","waterlogged"="true"]`)
		put(11, 1, 3, `minecraft:oak_slab ["type"="top","waterlogged"="true"]`)
		put(3, 1, 7, `minecraft:oak_fence ["waterlogged"="true"]`)
		put(7, 1, 7, `minecraft:oak_leaves ["waterlogged"="true"]`)
		put(11, 1, 7, "minecraft:seagrass []")
		put(13, 1, 7, "minecraft:kelp_plant []")
		put(13, 2, 7, `minecraft:kelp ["age"="0"]`)
		// 池内水与台阶水处于不同层，池壁与底面保持真实遮挡关系。
		box(2, 1, 10, 8, 2, 15, "minecraft:stone []")
		box(3, 1, 11, 7, 2, 14, `minecraft:water ["level"="0"]`)
		put(5, 1, 12, `minecraft:oak_slab ["type"="bottom","waterlogged"="true"]`)
		put(11, 1, 12, `minecraft:oak_slab ["type"="bottom","waterlogged"="true"]`)
		put(11, 2, 12, `minecraft:water ["level"="0"]`)
	case "foliage":
		box(2, 0, 2, 14, 1, 14, "minecraft:stone []")
		box(3, 1, 4, 13, 4, 7, `minecraft:oak_leaves ["distance"="7","persistent"="false"]`)
		box(4, 1, 7, 12, 3, 9, "minecraft:stone []")
		for x := 3; x < 13; x += 2 {
			put(x, 1, 11, "minecraft:short_grass []")
		}
		put(12, 1, 11, `minecraft:fire ["age"="0","east"="false","north"="false","south"="false","up"="false","west"="false"]`)
	case "fence_dirty":
		box(1, 0, 1, 15, 1, 15, "minecraft:stone []")
		// 保留默认连接状态，让开关展示 fixer 补出的横杆。
		box(3, 1, 3, 10, 2, 4, "minecraft:oak_fence []")
		put(10, 1, 3, "minecraft:stone []")
		box(3, 1, 7, 8, 2, 8, "minecraft:oak_fence []")
		box(7, 1, 8, 8, 2, 12, "minecraft:oak_fence []")
		put(7, 1, 12, "minecraft:stone []")
		put(12, 1, 7, "minecraft:oak_fence []")
		put(12, 1, 10, "minecraft:oak_fence []")
		put(3, 1, 12, "minecraft:oak_fence []")
	case "mixed":
		box(1, 0, 1, 15, 1, 15, "minecraft:stone []")
		box(2, 1, 3, 3, 5, 12, "minecraft:glass []")
		box(3, 1, 3, 6, 2, 8, `minecraft:oak_slab ["type"="bottom"]`)
		box(3, 1, 9, 6, 2, 12, `minecraft:oak_slab ["type"="top"]`)
		for x := 7; x < 14; x++ {
			// 预置连接用于对照，无需开启修复。
			put(x, 1, 3, fmt.Sprintf(
				`minecraft:oak_fence ["east"="%t","north"="false","south"="%t","west"="%t"]`,
				x < 13,
				x == 13,
				x > 7,
			))
			put(x, 1, 12, fmt.Sprintf(
				`minecraft:oak_fence ["east"="%t","north"="%t","south"="false","west"="%t"]`,
				x < 13,
				x == 13,
				x > 7,
			))
		}
		for z := 4; z < 12; z++ {
			put(13, 1, z, `minecraft:oak_fence ["east"="false","north"="true","south"="true","west"="false"]`)
		}
		box(10, 1, 5, 12, 4, 8, `minecraft:oak_leaves ["distance"="7","persistent"="false"]`)
		put(7, 1, 6, "minecraft:short_grass []")
		put(8, 1, 7, "minecraft:short_grass []")
		put(8, 1, 10, `minecraft:candle ["candles"="4","lit"="false"]`)
		put(4, 1, 13, `minecraft:chest ["facing"="south","type"="single"]`)
		put(7, 1, 9, `minecraft:red_bed ["part"="foot","facing"="north","occupied"="false"]`)
		put(7, 1, 8, `minecraft:red_bed ["part"="head","facing"="north","occupied"="false"]`)
	}
	return layer, nil
}
