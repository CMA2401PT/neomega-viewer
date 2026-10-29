package blockstate

import "neomega-assets/asset"

type SkippedState struct {
	Binding Binding
	Reason  string
}

type Renderer struct {
	Kind   asset.RendererKind `json:"kind"`
	Family string             `json:"family,omitempty"`
}

type Metadata struct {
	// Java 响应信息单独保存，几何使用者无需加载身份和渲染类别。
	Version   string
	Renderers map[asset.ID]Renderer
	Skipped   []SkippedState
}
