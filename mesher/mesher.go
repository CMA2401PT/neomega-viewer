package mesher

import blockgeometry "neomega-blocks/geometry"
import "neomega-viewer/mesher/geometry"

type Mesher[B blockgeometry.BlockID] struct {
	tables *blockgeometry.Tables[B]
}

func NewMesher[B blockgeometry.BlockID](tables *blockgeometry.Tables[B]) *Mesher[B] {
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
