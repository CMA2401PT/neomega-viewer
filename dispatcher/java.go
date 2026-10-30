package dispatcher

import (
	"neomega-blocks/ir"
	java "neomega-blocks/presets/java/v1"
	"neomega-viewer/blockactor"
	"neomega-viewer/effector"
	"neomega-viewer/splitlayer"
)

type Target uint8

const (
	Effect Target = iota + 1
	BlockActor
)

type Route struct {
	State  ir.IRBlock
	Target Target
}

type Routes map[splitlayer.JavaBlockHash]Route

func JavaRoutes(effectNames, actorNames []string) Routes {
	// 与 splitlayer 一样展开全集的两种含水变体，路由键保留完整 Java 状态。
	targets := map[string]Target{}
	for _, name := range effectNames {
		targets[name] = Effect
	}
	for _, name := range actorNames {
		targets[name] = BlockActor
	}
	routes := Routes{}
	for block := range java.JavaFullStates() {
		target, found := targets[block.NameWithNameSpace()]
		if !found {
			continue
		}
		block = block.Tidy()
		routes[block.Hash()] = Route{State: block, Target: target}
		for _, value := range []string{"true", "false"} {
			states := block.States().Set("waterlogged", value)
			variant := ir.IRBlock(block.NameWithNameSpace() + " " + string(states)).Tidy()
			routes[variant.Hash()] = Route{State: variant, Target: target}
		}
	}
	return routes
}

type Inputs struct {
	// 三条下游各自持有输入；actor 本体已从 Layers 中移除，水层仍保留。
	Layers  [][]splitlayer.JavaBlockHashRSL
	Effects []effector.Input
	Actors  []blockactor.Input
}

type Java struct {
	routes   Routes
	splitter *splitlayer.Splitter
	air      splitlayer.JavaBlockHashRSL
}

func NewJava(routes Routes) *Java {
	return &Java{
		routes:   routes,
		splitter: splitlayer.New(),
		air:      ir.IRBlock("minecraft:air []").Tidy().Hash(),
	}
}

func (d *Java) Dispatch(blocks []splitlayer.JavaBlockHash, nbt map[[3]uint8]string) Inputs {
	// blocks 按 (x<<8)|(z<<4)|y 排列 4096 格，必须是完整的 Java 状态；NBT 用格子坐标索引。
	result := Inputs{Layers: d.splitter.SplitSubchunk(blocks)}
	for index, hash := range blocks {
		route, found := d.routes[hash]
		if !found {
			continue
		}
		pos := [3]uint8{uint8(index >> 8), uint8(index & 15), uint8((index >> 4) & 15)}
		switch route.Target {
		case Effect:
			result.Effects = append(result.Effects, effector.Input{Pos: pos, State: route.State})
		case BlockActor:
			waterlogged := len(result.Layers) > 1 && result.Layers[1][index] != d.air
			result.Actors = append(result.Actors, blockactor.Input{
				Pos:         pos,
				State:       route.State,
				SNBT:        nbt[pos],
				Waterlogged: waterlogged,
			})
			result.Layers[0][index] = d.air
		}
	}
	return result
}
