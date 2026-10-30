package blockactor

import (
	"neomega-blocks/ir"
	"neomega-viewer/mesher/geometry"
)

type Input struct {
	Pos         [3]uint8
	State       ir.IRBlock
	SNBT        string
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
		mesh, err := d.handlers[input.State.NameWithNameSpace()].Build(input)
		if err != nil {
			return nil, err
		}
		// 复制后平移一次，处理器可以返回共享的格内模型。
		placed := geometry.Mesh{Quads: append([]geometry.Quad(nil), mesh.Quads...)}
		for i := range placed.Quads {
			for j := range placed.Quads[i].Positions {
				for axis, offset := range input.Pos {
					placed.Quads[i].Positions[j][axis] += float64(offset)
				}
			}
		}
		actors = append(actors, Actor{Pos: input.Pos, Mesh: placed})
	}
	return actors, nil
}
