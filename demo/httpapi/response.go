package httpapi

import (
	"neomega-blocks/ir"
	preset "neomega-blocks/presets/java/v1/geometry"
	"neomega-viewer/demo/singleblock"
	"neomega-viewer/mesher/geometry"
)

// StateResponse is the browser's display format. Core code uses IRBlock;
// expansion to a property object happens only when preparing an HTTP response.
type StateResponse struct {
	ID   uint32 `json:"id"`
	Java struct {
		Name       string         `json:"name"`
		Properties map[string]any `json:"properties"`
	} `json:"java"`
}

func stateResponse(block ir.IRBlock) StateResponse {
	result := StateResponse{ID: block.Hash()}
	result.Java.Name = block.NameWithNameSpace()
	result.Java.Properties = block.States().GoMap()
	return result
}

type BlockResponse struct {
	Binding  StateResponse    `json:"binding"`
	Renderer preset.Renderer  `json:"renderer"`
	Geometry geometry.Buffers `json:"geometry"`
}

func blockResponse(block *singleblock.Block) BlockResponse {
	return BlockResponse{Binding: stateResponse(block.State), Renderer: block.Renderer, Geometry: block.Mesh.Buffers()}
}
