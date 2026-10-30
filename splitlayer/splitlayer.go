package splitlayer

import (
	"slices"
	"sync"

	"neomega-blocks/ir"
	java "neomega-blocks/presets/java/v1"
)

type JavaBlockHash = uint32

// 完整 Java 状态经 Tidy 后的 hash，可能包含 waterlogged。

type JavaBlockHashRSL = uint32

// RSL 是单层渲染状态的 hash，可直接作为几何表的键。

func SplitBlock(block ir.IRBlock) []JavaBlockHashRSL {
	// 规则直接作用于完整状态，hash 表只是这套规则的预先展开。
	block = block.Tidy()
	states := block.States()
	water := false
	if _, found := states.Get("waterlogged"); found {
		water = states.CheckKey("waterlogged", "true")
		block = ir.IRBlock(block.NameWithNameSpace() + " " + string(states.Delete("waterlogged")))
	} else {
		switch block.NameWithNameSpace() {
		case "minecraft:seagrass", "minecraft:tall_seagrass", "minecraft:kelp", "minecraft:kelp_plant":
			water = true
		}
	}
	result := []JavaBlockHashRSL{block.Tidy().Hash()}
	if water {
		result = append(result, ir.IRBlock(`minecraft:water ["level"="0"]`).Tidy().Hash())
	}
	return result
}

type Splitter struct {
	// 规则表只读共享，拆出的切片由调用方持有。
	byHash map[JavaBlockHash][]JavaBlockHashRSL
	air    JavaBlockHashRSL
}

var bundledSplits = sync.OnceValues(func() (map[JavaBlockHash][]JavaBlockHashRSL, JavaBlockHashRSL) {
	// 全集没有 waterlogged，补齐两种取值才能识别调用方保留的含水状态。
	byHash := map[JavaBlockHash][]JavaBlockHashRSL{}
	for block := range java.JavaFullStates() {
		byHash[block.Tidy().Hash()] = SplitBlock(block)
		for _, value := range []string{"true", "false"} {
			states := block.States().Set("waterlogged", value)
			variant := ir.IRBlock(block.NameWithNameSpace() + " " + string(states)).Tidy()
			byHash[variant.Hash()] = SplitBlock(variant)
		}
	}
	return byHash, ir.IRBlock("minecraft:air []").Tidy().Hash()
})

func New() *Splitter {
	// 多个入口共享只读规则表，初始化时只展开一次状态全集。
	byHash, air := bundledSplits()
	return &Splitter{byHash: byHash, air: air}
}

func (s *Splitter) SplitHash(hash JavaBlockHash) []JavaBlockHashRSL {
	// 返回独立切片，调用方可修改结果而不影响共享规则表。
	if layers, found := s.byHash[hash]; found {
		return slices.Clone(layers)
	}
	return []JavaBlockHashRSL{hash}
}

func (s *Splitter) SplitSubchunk(blocks []JavaBlockHash) [][]JavaBlockHashRSL {
	// 输入为 4096 格；输出按层排列，每层仍用 (x<<8)|(z<<4)|y 寻址。
	// 层数由最深的格子决定，其余格子的缺层用 air 补齐。
	layers := [][]JavaBlockHashRSL{}
	for index, hash := range blocks {
		parts, found := s.byHash[hash]
		if !found {
			parts = []JavaBlockHashRSL{hash}
		}
		for len(layers) < len(parts) {
			layer := make([]JavaBlockHashRSL, len(blocks))
			for i := range layer {
				layer[i] = s.air
			}
			layers = append(layers, layer)
		}
		for layer, part := range parts {
			layers[layer][index] = part
		}
	}
	return layers
}
