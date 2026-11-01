// Package demo builds small, inspectable palette layers for the browser. These
// scene recipes are not part of the block parser or subchunk rendering library.
package demo

import (
	"fmt"
	"strings"

	"celler/layer_storage"
	"neomega-blocks/ir"
	java "neomega-blocks/presets/java/v1"
	nbt "neomega-nbt/presets/v1"
)

type Scene struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

var scenes = []Scene{
	{"block_entities", "方块实体", "附魔台的书特效与四朝向纹样陶罐，并列对照无 NBT 和含水陶罐。"},
	{"mixed", "形状与透明度", "石头、玻璃、半砖、栅栏、草木与特殊模型放在同一个子区块中。"},
	{"fence_dirty", "脏栅栏 · 未修复输入", "默认状态的栅栏排成直线和 L 形，端点挨着石头；开启修复可连接栏杆，三根孤立栅栏保持不连。"},
	{"special_java", "特殊方块 · Java 原版", "旗帜、附魔台、覆雪草方块、含水半砖、海晶灯、L 形台阶与多朝向的门、床、花卉及花盆植物，使用原始 Java 状态。"},
	{"special_nemc", "特殊方块 · NEMC → Java", "源数据是 Bedrock 方块层、第二层水和 block entity NBT，经 neomega-converter/bedrock 转成 Java，与 Java 原版布局逐格对照。"},
	{"solid", "填满石头", "4096 个石头方块。观察面剔除和网格合并各自减少了什么。"},
	{"glass", "玻璃与内部物体", "相同玻璃去掉内面，不同颜色保留界面；透过玻璃观察内部形状。"},
	{"fluids", "流体与纹理动画", "石池中的水与岩浆展示纹理动画；中央叠放同类流体，检查下层的满高度衔接。"},
	{"water_cube", "整块水", "大块源水内部的面应全部剔除，只有最顶层低于满格。"},
	{"waterlogged", "含水方块", "观察含水模型与水的共面裁切、植物和树叶的重叠，以及水池与叠水的跨层衔接。"},
	{"slabs", "半砖交界", "半砖与完整方块相邻，只剔除实际覆盖的部分。"},
	{"foliage", "树叶与交叉平面", "透过镂空贴图观察后面的石头，并检查草和火焰的平面外形。"},
	{"checker", "交错材质", "内部面仍会剔除，纹理不同的外表面保持分开。"},
	{"empty", "空气", "默认填充值也是数据；空气返回空网格。"},
}

func List() []Scene { return append([]Scene(nil), scenes...) }

type Subchunk struct {
	Blocks *layer_storage.Palette[uint32]
	NBT    map[[3]uint8]nbt.Compound
}

func Build(name string) (*layer_storage.Palette[uint32], error) {
	// 旧入口只取方块层，保持网格预览和下载的输入形式。
	scene, err := BuildWithNBT(name)
	if err != nil {
		return nil, err
	}
	return scene.Blocks, nil
}

func BuildWithNBT(name string) (*Subchunk, error) {
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
	air := blockID("minecraft:air []")
	layer := layer_storage.MakePaletteStorage(air)
	put := func(x, y, z int, java string) {
		layer.Set(layer_storage.At(x, y, z), blockID(java))
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

	nbt := map[[3]uint8]string{}
	switch name {
	case "block_entities":
		box(1, 0, 1, 15, 1, 15, "minecraft:stone []")
		put(3, 1, 3, "minecraft:enchanting_table []")
		for i, facing := range []string{"north", "east", "south", "west"} {
			x := 3 + i*3
			put(x, 1, 7, fmt.Sprintf(`minecraft:decorated_pot ["facing"="%s"]`, facing))
			// 取自 1.21.4 存档的原样数据：front 为举臂纹样，便于和游戏内对照朝向。
			nbt[[3]uint8{uint8(x), 1, 7}] = `{sherds:{back:{id:"minecraft:angler_pottery_sherd"},left:{id:"minecraft:archer_pottery_sherd"},right:{id:"minecraft:blade_pottery_sherd"},front:{id:"minecraft:arms_up_pottery_sherd"}}}`
		}
		put(5, 1, 11, `minecraft:decorated_pot ["facing"="north","cracked"="true"]`)
		put(10, 1, 11, `minecraft:decorated_pot ["facing"="east","waterlogged"="true"]`)
		// 含水陶罐用 1.20 早期的列表格式，同一条渲染路径兼顾两种 NBT。
		nbt[[3]uint8{10, 1, 11}] = `{sherds:["minecraft:angler_pottery_sherd","minecraft:archer_pottery_sherd","minecraft:blade_pottery_sherd","minecraft:arms_up_pottery_sherd"]}`
	case "empty":
	case "special_java":
		box(1, 0, 1, 15, 1, 15, "minecraft:stone []")
		put(3, 1, 3, `minecraft:white_banner ["rotation"="0"]`)
		put(7, 1, 3, "minecraft:enchanting_table []")
		put(11, 1, 3, `minecraft:sunflower ["half"="lower"]`)
		put(11, 2, 3, `minecraft:sunflower ["half"="upper"]`)
		put(3, 1, 7, `minecraft:grass_block ["snowy"="true"]`)
		// 含雪草方块的灰度顶面在原版中由上方雪层遮住。
		put(3, 2, 7, `minecraft:snow ["layers"="1"]`)
		put(7, 1, 7, `minecraft:oak_slab ["type"="bottom","waterlogged"="true"]`)
		put(11, 1, 7, "minecraft:sea_lantern []")
		// north 朝向负 Z，床头在床尾北侧；门与植物均保留上下两格。
		put(3, 1, 11, `minecraft:red_bed ["part"="foot","facing"="north","occupied"="false"]`)
		put(3, 1, 10, `minecraft:red_bed ["part"="head","facing"="north","occupied"="false"]`)
		put(7, 1, 11, `minecraft:oak_door ["half"="lower","facing"="north","hinge"="left","open"="false","powered"="false"]`)
		put(7, 2, 11, `minecraft:oak_door ["half"="upper","facing"="north","hinge"="left","open"="false","powered"="false"]`)
		put(11, 1, 11, "minecraft:potted_cactus []")
		put(13, 1, 2, `minecraft:spruce_door ["half"="lower","facing"="east","hinge"="right","open"="false","powered"="false"]`)
		put(13, 2, 2, `minecraft:spruce_door ["half"="upper","facing"="east","hinge"="right","open"="false","powered"="false"]`)
		put(13, 1, 5, `minecraft:birch_door ["half"="lower","facing"="south","hinge"="left","open"="true","powered"="false"]`)
		put(13, 2, 5, `minecraft:birch_door ["half"="upper","facing"="south","hinge"="left","open"="true","powered"="false"]`)
		put(13, 1, 8, `minecraft:iron_door ["half"="lower","facing"="west","hinge"="right","open"="false","powered"="false"]`)
		put(13, 2, 8, `minecraft:iron_door ["half"="upper","facing"="west","hinge"="right","open"="false","powered"="false"]`)
		// 床头沿 facing 方向相邻，横向与纵向摆放便于对照朝向转换。
		put(2, 1, 13, `minecraft:blue_bed ["part"="foot","facing"="east","occupied"="false"]`)
		put(3, 1, 13, `minecraft:blue_bed ["part"="head","facing"="east","occupied"="false"]`)
		put(5, 1, 13, `minecraft:yellow_bed ["part"="foot","facing"="south","occupied"="false"]`)
		put(5, 1, 14, `minecraft:yellow_bed ["part"="head","facing"="south","occupied"="false"]`)
		put(9, 1, 13, `minecraft:lime_bed ["part"="foot","facing"="west","occupied"="false"]`)
		put(8, 1, 13, `minecraft:lime_bed ["part"="head","facing"="west","occupied"="false"]`)
		put(3, 1, 5, `minecraft:pink_petals ["facing"="north","flower_amount"="1"]`)
		put(5, 1, 5, `minecraft:pink_petals ["facing"="east","flower_amount"="2"]`)
		put(7, 1, 5, `minecraft:pink_petals ["facing"="south","flower_amount"="3"]`)
		put(9, 1, 5, `minecraft:pink_petals ["facing"="west","flower_amount"="4"]`)
		put(13, 1, 11, `minecraft:sunflower ["half"="lower"]`)
		put(13, 2, 11, `minecraft:sunflower ["half"="upper"]`)
		put(13, 1, 14, "minecraft:poppy []")
		put(9, 1, 8, `minecraft:oak_stairs ["facing"="west","half"="bottom","shape"="straight"]`)
		// 前方台阶朝西，是本块朝向逆时针一格，原版取 outer_left，高出的一角在西北与两侧相接。
		put(9, 1, 9, `minecraft:oak_stairs ["facing"="north","half"="bottom","shape"="outer_left"]`)
		put(8, 1, 9, `minecraft:oak_stairs ["facing"="north","half"="bottom","shape"="straight"]`)
	case "special_nemc":
		return buildSpecialNEMC()
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
	case "water_cube":
		box(1, 0, 1, 15, 1, 15, "minecraft:stone []")
		box(2, 1, 2, 6, 5, 6, `minecraft:water ["level"="0"]`)
		box(7, 1, 3, 8, 5, 4, `minecraft:water ["level"="0"]`)
		// 玻璃外壳中填水，留下四壁；底部沿用石头地板，顶部敞开。
		box(9, 1, 1, 15, 5, 7, "minecraft:glass []")
		box(10, 1, 2, 14, 5, 6, `minecraft:water ["level"="0"]`)
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
		// 单侧相接，便于对照有水和无水的那几面。
		put(3, 1, 4, `minecraft:water ["level"="0"]`)
		put(7, 1, 4, `minecraft:water ["level"="0"]`)
		put(11, 1, 4, `minecraft:water ["level"="0"]`)
		put(3, 1, 8, `minecraft:water ["level"="0"]`)
		put(7, 1, 8, `minecraft:water ["level"="0"]`)
		put(11, 1, 8, `minecraft:water ["level"="0"]`)
		put(13, 1, 8, `minecraft:water ["level"="0"]`)
		put(13, 2, 8, `minecraft:water ["level"="0"]`)
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
	entities, err := decodeSceneNBT(nbt)
	if err != nil {
		return nil, err
	}
	return &Subchunk{Blocks: layer, NBT: entities}, nil
}

func decodeSceneNBT(texts map[[3]uint8]string) (map[[3]uint8]nbt.Compound, error) {
	// 场景配方保留易读的 SNBT，进入渲染前只在 demo 一侧解析一次。
	entities := make(map[[3]uint8]nbt.Compound, len(texts))
	for pos, text := range texts {
		var value nbt.Compound
		if err := nbt.DecodeFromSNBT(text, &value, nbt.OptDecodeFromSNBT{}); err != nil {
			return nil, err
		}
		entities[pos] = value
	}
	return entities, nil
}
