package blockactor

import (
	"fmt"

	jmodel "neomega-blocks/geometry/java_block_model"
	"neomega-blocks/geometry/material"
	"neomega-blocks/ir"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer/mesher/geometry"
)

type Input struct {
	Pos         [3]uint8
	State       ir.IRBlock
	NBT         nbt.Compound
	Waterlogged bool
}

type Actor struct {
	// Pos 只标记归属，Mesh 顶点已经位于子区块局部坐标。
	Pos  [3]uint8
	Mesh geometry.Mesh
}

type Handler interface {
	// 处理器返回按状态旋转的格内几何，格子平移统一由 Dispatcher 执行。
	BlockNames() []string
	Build(input Input) (geometry.Mesh, error)
}

type Dispatcher struct {
	handlers map[string]Handler
}

func New(handlers ...Handler) *Dispatcher {
	d := &Dispatcher{handlers: map[string]Handler{}}
	for _, handler := range handlers {
		for _, name := range handler.BlockNames() {
			d.handlers[name] = handler
		}
	}
	return d
}

func (d *Dispatcher) BlockNames() []string {
	names := make([]string, 0, len(d.handlers))
	for name := range d.handlers {
		names = append(names, name)
	}
	return names
}

func (d *Dispatcher) Build(inputs []Input) ([]Actor, error) {
	actors := make([]Actor, 0, len(inputs))
	for _, input := range inputs {
		actor, err := d.BuildOne(input)
		if err != nil {
			return nil, err
		}
		actors = append(actors, actor)
	}
	return actors, nil
}

func (d *Dispatcher) BuildOne(input Input) (Actor, error) {
	// 复制后平移一次，处理器可以返回共享的格内模型。
	mesh, err := d.handlers[input.State.NameWithNameSpace()].Build(input)
	if err != nil {
		return Actor{}, err
	}
	placed := geometry.Mesh{Quads: append([]geometry.Quad(nil), mesh.Quads...)}
	for i := range placed.Quads {
		for j := range placed.Quads[i].Positions {
			for axis, offset := range input.Pos {
				placed.Quads[i].Positions[j][axis] += float64(offset)
			}
		}
	}
	return Actor{Pos: input.Pos, Mesh: placed}, nil
}

// 贴图名带 minecraft: 命名空间，查询结果包含配套图集区域和材质属性。
type SurfaceLookup func(texture string) (material.Surface, error)

func AtlasSurfaces(atlas *jmodel.Atlas) SurfaceLookup {
	// actor 与静态模型使用配套图集，统一保留区域与动画参数。
	return func(texture string) (material.Surface, error) {
		entry, found := atlas.Textures[jmodel.ResourceID(texture)]
		if !found {
			return material.Surface{}, fmt.Errorf("missing actor texture %s", texture)
		}
		return material.Surface{
			Texture: texture,
			Region: [4]float32{
				float32(entry.U),
				float32(entry.V),
				float32(entry.SU),
				float32(entry.SV),
			},
			Mode:      material.Opaque,
			Opacity:   1,
			Frames:    entry.Frames,
			FrameTime: entry.FrameTime,
		}, nil
	}
}
