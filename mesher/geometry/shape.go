package geometry

type BoundaryRect struct {
	// 保留面索引，剔面时才能取得覆盖面的材质。
	Quad int
	Min  Vec2
	Max  Vec2
}

type Shape struct {
	// 共享烘焙结果只读，不携带状态属性或模型选择规则。
	Quads []Quad
	// 同类透明介质共用非零编号，避免渲染时根据方块名判断消面。
	CullGroup uint32
	// 顺序为 west、east、down、up、north、south，与现有剔面一致。
	// 法向轴为 axis 时，矩形坐标轴依次为 (axis+1)%3、(axis+2)%3。
	Boundary [6][]BoundaryRect
}
