package main

import (
	"context"
	"math"
	"math/rand/v2"

	"celler/layer_storage"
	"neomega-blocks/ir"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer/dispatcher"
)

var (
	air   = ir.IRBlock("minecraft:air []")
	stone = ir.IRBlock("minecraft:stone []")
	dirt  = ir.IRBlock("minecraft:dirt []")
	grass = ir.IRBlock(`minecraft:grass_block ["snowy"="false"]`)
	pots  = []ir.IRBlock{
		ir.IRBlock(`minecraft:decorated_pot ["cracked"="false","facing"="north","waterlogged"="false"]`),
		ir.IRBlock(`minecraft:decorated_pot ["cracked"="false","facing"="east","waterlogged"="false"]`),
		ir.IRBlock(`minecraft:decorated_pot ["cracked"="false","facing"="south","waterlogged"="false"]`),
		ir.IRBlock(`minecraft:decorated_pot ["cracked"="false","facing"="west","waterlogged"="false"]`),
	}
	sherds = []string{
		"minecraft:angler_pottery_sherd",
		"minecraft:archer_pottery_sherd",
		"minecraft:blade_pottery_sherd",
		"minecraft:arms_up_pottery_sherd",
	}
)

// surfaceHeight 根据世界方块坐标生成低丘，跨子区块边界仍然连续。
func surfaceHeight(x, z int64) int {
	return 5 + int(math.Floor(2*math.Sin(float64(x)/17)+2*math.Cos(float64(z)/23)))
}

// generatedWorld 按坐标重建地形和装饰，不持有已生成的地图。
type generatedWorld struct{}

// Section 提供指定位置的方块与实体数据；地形和装饰都位于 Y=0 子区块。
func (generatedWorld) Section(_ context.Context, pos dispatcher.SectionPos) (*dispatcher.Section, error) {
	if pos.Y != 0 {
		return nil, nil
	}
	section := &dispatcher.Section{
		Blocks:   layer_storage.MakePaletteStorage(air),
		Entities: map[dispatcher.CellPos]nbt.Compound{},
	}
	originX, originZ := int64(pos.X)*16, int64(pos.Z)*16
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			top := surfaceHeight(originX+int64(x), originZ+int64(z))
			for y := 0; y <= top; y++ {
				state := stone
				if y == top {
					state = grass
				} else if y == top-1 {
					state = dirt
				}
				section.Blocks.Set(layer_storage.At(x, y, z), state)
			}
		}
	}
	// 以区块坐标作为随机种子，卸载后再回来仍是同一组摆设。
	random := rand.New(rand.NewPCG(uint64(int64(pos.X)), uint64(int64(pos.Z))))
	x, z := 2+random.IntN(5), 2+random.IntN(12)
	y := surfaceHeight(originX+int64(x), originZ+int64(z)) + 1
	section.Blocks.Set(layer_storage.At(x, y, z), ir.IRBlock("minecraft:enchanting_table []"))
	x, z = 10+random.IntN(4), 2+random.IntN(12)
	y = surfaceHeight(originX+int64(x), originZ+int64(z)) + 1
	section.Blocks.Set(layer_storage.At(x, y, z), pots[random.IntN(len(pots))])
	items := make([]string, 4)
	for side := range items {
		items[side] = sherds[random.IntN(len(sherds))]
	}
	section.Entities[dispatcher.CellPos{uint8(x), uint8(y), uint8(z)}] = nbt.Compound{"sherds": items}
	return section, nil
}
