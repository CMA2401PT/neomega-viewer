package dispatcher

import (
	"context"
	"fmt"

	"celler/layer_storage"
	blockgeometry "neomega-blocks/geometry"
	jmodel "neomega-blocks/geometry/java_block_model"
	"neomega-blocks/ir"
	java "neomega-blocks/presets/java/v1"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer/blockactor"
	"neomega-viewer/blockactor/decoratedpot"
	"neomega-viewer/effector"
	"neomega-viewer/merger"
	"neomega-viewer/mesher"
	"neomega-viewer/splitlayer"
)

// section 内按 x、y、z 定位方块实体，与格内几何的平移一致。
type CellPos = [3]uint8

type Section struct {
	// 完整 Java 状态保留含水属性，诊断和 actor 均使用拆层前的状态。
	Blocks   *layer_storage.Palette[ir.IRBlock]
	Entities map[CellPos]nbt.Compound
}

// 顺序为西、东、下、上、北、南；nil 表示空空间。
type Neighbours [6]*Section

type DiagnosticKind string

const (
	MissingModel DiagnosticKind = "missing_model"
	ActorError   DiagnosticKind = "actor_error"
)

type Diagnostic struct {
	// 方块级问题保留完整状态，调用方负责换算世界坐标和选择展示方式。
	Kind  DiagnosticKind
	Pos   CellPos
	State ir.IRBlock
	Err   error
}

type Pipeline struct {
	// 组装结果只读共享，每次 Build 单独产生一个子区块快照。
	mesher      *mesher.Mesher[uint32]
	effects     *effector.Effector
	actors      *blockactor.Dispatcher
	effectNames map[string]bool
	actorNames  map[string]bool
	missing     *blockgeometry.Shape
}

type Options struct {
	Mesh mesher.Options
}

type Result struct {
	Frame       merger.Frame
	Stats       mesher.Stats
	Diagnostics []Diagnostic
}

func NewPipeline(
	tables *blockgeometry.Tables[uint32],
	effects *effector.Effector,
	actors *blockactor.Dispatcher,
	missing *blockgeometry.Shape,
) *Pipeline {
	// 从已启用的处理器生成路由，认领范围和实际可调用的处理器保持一致。
	p := &Pipeline{
		mesher:      mesher.NewMesher(tables),
		effects:     effects,
		actors:      actors,
		effectNames: map[string]bool{},
		actorNames:  map[string]bool{},
		missing:     missing,
	}
	for _, name := range effects.BlockNames() {
		p.effectNames[name] = true
	}
	for _, name := range actors.BlockNames() {
		p.actorNames[name] = true
	}
	return p
}

func NewJavaPipeline(
	tables *blockgeometry.Tables[uint32],
	atlas *jmodel.Atlas,
	missing *blockgeometry.Shape,
) *Pipeline {
	// 共享应用默认启用的特效和 actor；陶罐换纹样需要配套图集。
	actors := blockactor.New()
	if atlas != nil {
		actors = blockactor.New(decoratedpot.New(tables, blockactor.AtlasSurfaces(atlas)))
	}
	return NewPipeline(
		tables,
		effector.New(map[string]string{"minecraft:enchanting_table": "enchanting_table_book"}),
		actors,
		missing,
	)
}

type prepared struct {
	// 同一份拆层供中心与邻居复用，邻居的 actor 本体保守地不提供覆盖。
	layers     [][]uint32
	neighbours [][]uint32
	states     []ir.IRBlock
	effects    []effector.Input
	actors     []blockactor.Input
}

var air = ir.IRBlock("minecraft:air []").Hash()

func (p *Pipeline) prepare(section Section) *prepared {
	// 按调色板项拆层与路由，再展开到 4096 格，兼容预置全集以外的 Java 状态。
	result := &prepared{}
	if section.Blocks == nil {
		return result
	}
	palette := section.Blocks.GetUsedPalette()
	parts := make([][]uint32, len(palette))
	effects := make([]bool, len(palette))
	actors := make([]bool, len(palette))
	depth := 1
	for i, block := range palette {
		parts[i] = splitlayer.SplitBlock(block)
		name := block.NameWithNameSpace()
		effects[i], actors[i] = p.effectNames[name], p.actorNames[name]
		depth = max(depth, len(parts[i]))
	}
	result.layers = make([][]uint32, depth)
	for layer := range result.layers {
		result.layers[layer] = make([]uint32, 4096)
		for i := range result.layers[layer] {
			result.layers[layer][i] = air
		}
	}
	result.states = make([]ir.IRBlock, 4096)
	for i := range result.states {
		pos := CellPos{uint8(i >> 8), uint8(i & 15), uint8(i >> 4 & 15)}
		index := section.Blocks.GetIndex(layer_storage.At(pos[0], pos[1], pos[2]))
		state := palette[index]
		result.states[i] = state
		for layer, id := range parts[index] {
			result.layers[layer][i] = id
		}
		if effects[index] {
			result.effects = append(result.effects, effector.Input{Pos: pos, State: state})
		}
		if actors[index] {
			result.actors = append(result.actors, blockactor.Input{
				Pos:         pos,
				State:       state,
				NBT:         section.Entities[pos],
				Waterlogged: len(parts[index]) > 1 && parts[index][1] != air,
			})
		}
	}
	result.neighbours = result.layers
	if len(result.actors) > 0 {
		result.neighbours = cloneBody(result.layers)
		for _, actor := range result.actors {
			result.neighbours[0][cellIndex(actor.Pos)] = air
		}
	}
	return result
}

func cellIndex(pos CellPos) int { return int(pos[0])<<8 | int(pos[2])<<4 | int(pos[1]) }

func cloneBody(layers [][]uint32) [][]uint32 {
	// 只修改本体层，其他层继续共享只读切片。
	out := append([][]uint32(nil), layers...)
	out[0] = append([]uint32(nil), layers[0]...)
	return out
}

func (p *Pipeline) Build(section Section, neighbours Neighbours, options Options) (*Result, error) {
	// 单 section 与批量构建共用准备和构建逻辑。
	var adjacent [6]*prepared
	for side, neighbour := range neighbours {
		if neighbour != nil {
			adjacent[side] = p.prepare(*neighbour)
		}
	}
	return p.build(p.prepare(section), adjacent, options)
}

func (p *Pipeline) build(section *prepared, neighbours [6]*prepared, options Options) (*Result, error) {
	// actor 成功才移除静态本体；失败只影响当前格，水层始终保留。
	result := &Result{}
	layers := section.layers
	if len(section.actors) > 0 {
		layers = cloneBody(layers)
	}
	for _, input := range section.actors {
		actor, err := p.actors.BuildOne(input)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Kind:  ActorError,
				Pos:   input.Pos,
				State: input.State,
				Err:   err,
			})
			continue
		}
		result.Frame.Actors = append(result.Frame.Actors, actor)
		layers[0][cellIndex(input.Pos)] = air
	}
	var adjacent [6][][]uint32
	for side, neighbour := range neighbours {
		if neighbour != nil {
			adjacent[side] = neighbour.neighbours
		}
	}
	options.Mesh.MissingShape = p.missing
	static, err := p.mesher.MeshWithNeighbours(layers, adjacent, options.Mesh)
	if err != nil {
		return nil, err
	}
	seen := map[CellPos]bool{}
	for _, missing := range static.Missing {
		if !seen[missing.Pos] {
			seen[missing.Pos] = true
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Kind:  MissingModel,
				Pos:   missing.Pos,
				State: section.states[cellIndex(missing.Pos)],
			})
		}
	}
	result.Frame.Mesh = static.Mesh
	result.Frame.Effects = p.effects.Build(section.effects)
	result.Stats = static.Stats
	return result, nil
}

func BlocksFromHashes(blocks []splitlayer.JavaBlockHash) (*layer_storage.Palette[ir.IRBlock], error) {
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

type SectionPos struct{ X, Y, Z int32 }

type SectionSource interface {
	// nil, nil 表示该位置不存在，按空气处理。
	Section(ctx context.Context, pos SectionPos) (*Section, error)
}

type PositionedResult struct {
	Pos    SectionPos
	Result *Result
}

func (p *Pipeline) BuildSections(
	ctx context.Context,
	source SectionSource,
	positions []SectionPos,
	options Options,
) ([]PositionedResult, error) {
	// 请求位置与六向邻居在调用内只读取和准备一次，仅输出请求位置，不存在的位置产生空快照。
	cache := map[SectionPos]*prepared{}
	load := func(pos SectionPos) (*prepared, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if value, found := cache[pos]; found {
			return value, nil
		}
		section, err := source.Section(ctx, pos)
		if err != nil {
			return nil, err
		}
		value := &prepared{}
		if section != nil {
			value = p.prepare(*section)
		}
		cache[pos] = value
		return value, nil
	}
	var results []PositionedResult
	for _, pos := range positions {
		section, err := load(pos)
		if err != nil {
			return nil, err
		}
		var neighbours [6]*prepared
		for side, delta := range [6]SectionPos{{X: -1}, {X: 1}, {Y: -1}, {Y: 1}, {Z: -1}, {Z: 1}} {
			neighbours[side], err = load(SectionPos{
				X: pos.X + delta.X,
				Y: pos.Y + delta.Y,
				Z: pos.Z + delta.Z,
			})
			if err != nil {
				return nil, err
			}
		}
		result, err := p.build(section, neighbours, options)
		if err != nil {
			return nil, err
		}
		results = append(results, PositionedResult{Pos: pos, Result: result})
	}
	return results, nil
}
