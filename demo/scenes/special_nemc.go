package demo

import (
	"celler"
	cellerbedrock "celler/bedrock"
	"celler/layer_storage"
	"neomega-blocks/ir"
	java "neomega-blocks/presets/java/v1"
	bedrock "neomega-converter-core/be2je"
	nbt "neomega-nbt/presets/v1"
)

func buildSpecialNEMC() (*Subchunk, error) {
	// 直接保留 Bedrock 的两层与 NBT，让转换器读取源数据中的颜色、植物和水。
	air := ir.IRBlock("minecraft:air []").Tidy()
	layers := [][]ir.IRBlock{make([]ir.IRBlock, 4096), make([]ir.IRBlock, 4096)}
	for _, layer := range layers {
		for index := range layer {
			layer[index] = air
		}
	}
	put := func(x, y, z int, source string) {
		layers[0][(x<<8)|(z<<4)|y] = ir.IRBlock(source).Tidy()
	}
	for x := 1; x < 15; x++ {
		for z := 1; z < 15; z++ {
			put(x, 0, z, "minecraft:stone []")
		}
	}
	put(3, 1, 3, `minecraft:standing_banner ["ground_sign_direction"=0]`)
	put(7, 1, 3, "minecraft:enchanting_table []")
	put(11, 1, 3, `minecraft:sunflower ["upper_block_bit"=false]`)
	put(11, 2, 3, `minecraft:sunflower ["upper_block_bit"=true]`)
	put(3, 1, 7, "minecraft:grass_block []")
	put(3, 2, 7, `minecraft:snow_layer ["covered_bit"=false,"height"=0]`)
	put(7, 1, 7, `minecraft:oak_slab ["minecraft:vertical_half"="bottom"]`)
	layers[1][(7<<8)|(7<<4)|1] = ir.IRBlock(`minecraft:water ["liquid_depth"=0]`).Tidy()
	put(11, 1, 7, "minecraft:sea_lantern []")
	put(3, 1, 11, `minecraft:bed ["direction"=2,"head_piece_bit"=false,"occupied_bit"=false]`)
	put(3, 1, 10, `minecraft:bed ["direction"=2,"head_piece_bit"=true,"occupied_bit"=false]`)
	put(7, 1, 11, `minecraft:wooden_door ["direction"=3,"door_hinge_bit"=false,"open_bit"=false,"upper_block_bit"=false]`)
	put(7, 2, 11, `minecraft:wooden_door ["direction"=0,"door_hinge_bit"=false,"open_bit"=false,"upper_block_bit"=true]`)
	put(11, 1, 11, `minecraft:flower_pot ["update_bit"=false]`)
	put(13, 1, 2, `minecraft:spruce_door ["direction"=0,"door_hinge_bit"=false,"open_bit"=false,"upper_block_bit"=false]`)
	put(13, 2, 2, `minecraft:spruce_door ["direction"=0,"door_hinge_bit"=true,"open_bit"=false,"upper_block_bit"=true]`)
	put(13, 1, 5, `minecraft:birch_door ["direction"=1,"door_hinge_bit"=false,"open_bit"=true,"upper_block_bit"=false]`)
	put(13, 2, 5, `minecraft:birch_door ["direction"=0,"door_hinge_bit"=false,"open_bit"=false,"upper_block_bit"=true]`)
	put(13, 1, 8, `minecraft:iron_door ["direction"=2,"door_hinge_bit"=false,"open_bit"=false,"upper_block_bit"=false]`)
	put(13, 2, 8, `minecraft:iron_door ["direction"=0,"door_hinge_bit"=true,"open_bit"=false,"upper_block_bit"=true]`)
	put(2, 1, 13, `minecraft:bed ["direction"=3,"head_piece_bit"=false,"occupied_bit"=false]`)
	put(3, 1, 13, `minecraft:bed ["direction"=3,"head_piece_bit"=true,"occupied_bit"=false]`)
	put(5, 1, 13, `minecraft:bed ["direction"=0,"head_piece_bit"=false,"occupied_bit"=false]`)
	put(5, 1, 14, `minecraft:bed ["direction"=0,"head_piece_bit"=true,"occupied_bit"=false]`)
	put(9, 1, 13, `minecraft:bed ["direction"=1,"head_piece_bit"=false,"occupied_bit"=false]`)
	put(8, 1, 13, `minecraft:bed ["direction"=1,"head_piece_bit"=true,"occupied_bit"=false]`)
	put(3, 1, 5, `minecraft:pink_petals ["growth"=0,"minecraft:cardinal_direction"="north"]`)
	put(5, 1, 5, `minecraft:pink_petals ["growth"=1,"minecraft:cardinal_direction"="east"]`)
	put(7, 1, 5, `minecraft:pink_petals ["growth"=2,"minecraft:cardinal_direction"="south"]`)
	put(9, 1, 5, `minecraft:pink_petals ["growth"=3,"minecraft:cardinal_direction"="west"]`)
	put(13, 1, 11, `minecraft:sunflower ["upper_block_bit"=false]`)
	put(13, 2, 11, `minecraft:sunflower ["upper_block_bit"=true]`)
	put(13, 1, 14, "minecraft:poppy []")
	put(9, 1, 8, `minecraft:oak_stairs ["upside_down_bit"=false,"weirdo_direction"=1]`)
	put(9, 1, 9, `minecraft:oak_stairs ["upside_down_bit"=false,"weirdo_direction"=3]`)
	put(8, 1, 9, `minecraft:oak_stairs ["upside_down_bit"=false,"weirdo_direction"=3]`)

	entities := map[[3]uint8]string{
		// 床头尾各有颜色，花盆植物独立于方块状态，必须随源方块传入。
		{3, 1, 11}:  `{id:"Bed",color:14b}`,
		{3, 1, 10}:  `{id:"Bed",color:14b}`,
		{2, 1, 13}:  `{id:"Bed",color:11b}`,
		{3, 1, 13}:  `{id:"Bed",color:11b}`,
		{5, 1, 13}:  `{id:"Bed",color:4b}`,
		{5, 1, 14}:  `{id:"Bed",color:4b}`,
		{9, 1, 13}:  `{id:"Bed",color:5b}`,
		{8, 1, 13}:  `{id:"Bed",color:5b}`,
		{11, 1, 11}: `{id:"FlowerPot",PlantBlock:{name:"minecraft:cactus",states:{age:0},version:18168865}}`,
	}
	source := cellerbedrock.NewChunk(celler.ChunkPos{}, 0)
	section := &cellerbedrock.Section{Y: 0, Version: 9}
	for _, states := range layers {
		layer := cellerbedrock.NewLayer(cellerbedrock.Block{State: air, Version: 18168865})
		for index, state := range states {
			layer.Blocks.Set(
				layer_storage.At(index>>8, index&15, index>>4&15),
				cellerbedrock.Block{State: state, Version: 18168865},
			)
		}
		section.Layers = append(section.Layers, layer)
	}
	if err := source.SetSection(section); err != nil {
		return nil, err
	}
	var documents []nbt.Document
	for pos, text := range entities {
		var value nbt.Compound
		if err := nbt.DecodeFromSNBT(text, &value, nbt.OptDecodeFromSNBT{}); err != nil {
			return nil, err
		}
		value["x"], value["y"], value["z"] = int32(pos[0]), int32(pos[1]), int32(pos[2])
		documents = append(documents, nbt.Document{Value: value})
	}
	if err := source.SetBlockEntities(documents); err != nil {
		return nil, err
	}
	converted, err := bedrock.ConvertChunk(source)
	if err != nil {
		return nil, err
	}
	layer := layer_storage.ConvertPalette(converted.Sections[0].Blocks, java.BlockToHash)
	decoded := make(map[[3]uint8]nbt.Compound, len(converted.BlockEntities))
	for _, entity := range converted.BlockEntities {
		pos := layer_storage.At(entity.Pos.X, entity.Pos.Y, entity.Pos.Z)
		decoded[[3]uint8{pos.X, pos.Y, pos.Z}] = entity.NBT
	}
	return &Subchunk{Blocks: layer, NBT: decoded}, nil
}
