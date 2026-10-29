package mesher

import "neomega-viewer/geometry"

type BlockID interface {
	// 调用者决定数值的含义；自定义 uint32/uint64 类型也可以作为表键。
	~uint32 | ~uint64
}

type Mesher[B BlockID] struct {
	tables *Tables[B]
}

func NewMesher[B BlockID](tables *Tables[B]) *Mesher[B] {
	return &Mesher[B]{tables: tables}
}

func (m *Mesher[B]) MeshBlock(id B) (geometry.Mesh, error) {
	ids, err := m.tables.ModelIDs(id)
	if err != nil {
		return geometry.Mesh{}, err
	}
	mesh := geometry.Mesh{Quads: []geometry.Quad{}}
	for _, modelID := range ids {
		shape, err := m.tables.Shape(modelID)
		if err != nil {
			return geometry.Mesh{}, err
		}
		// 输出独立网格，调用者修改它不会改变共享表。
		mesh.Quads = append(mesh.Quads, shape.Quads...)
	}
	return mesh, nil
}
