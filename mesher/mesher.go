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
	parts, _, err := queryGeometry(m.tables, id, 0, nil)
	if err != nil {
		return geometry.Mesh{}, err
	}
	mesh := geometry.Mesh{Quads: []geometry.Quad{}}
	for _, part := range parts {
		// 输出独立网格，调用者修改它不会改变共享表。
		mesh.Quads = append(mesh.Quads, part.Quads...)
	}
	return mesh, nil
}
