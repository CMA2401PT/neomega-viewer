package meshing

import "neomega-viewer/mesher/geometry"

func BoundaryCoverage(mesh geometry.Mesh) [6][]geometry.BoundaryRect {
	// 建表与现有剔面共用矩形识别，避免旋转误差和局部覆盖的判定分叉。
	var result [6][]geometry.BoundaryRect
	for index, quad := range mesh.Quads {
		rect, ok := inspect(quad)
		if !ok {
			continue
		}
		if side := boundary(rect); side >= 0 {
			result[side] = append(result[side], geometry.BoundaryRect{
				Quad: index,
				Min:  geometry.Vec2(rect.lo),
				Max:  geometry.Vec2(rect.hi),
			})
		}
	}
	return result
}
