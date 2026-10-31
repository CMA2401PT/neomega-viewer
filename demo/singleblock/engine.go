// Package singleblock renders an isolated block without constructing a section.
package singleblock

import (
	"fmt"

	"neomega-blocks/ir"
	java "neomega-blocks/presets/java/v1"
	blockmesh "neomega-blocks/presets/java/v1/geometry"
	"neomega-viewer/mesher"
	"neomega-viewer/mesher/geometry"
)

type Engine struct {
	*blockmesh.Preset
	mesher *mesher.Mesher[uint32]
}

type Block struct {
	State    ir.IRBlock
	Renderer blockmesh.Renderer
	Mesh     geometry.Mesh
}

func New(resolver *blockmesh.Preset) *Engine {
	return &Engine{Preset: resolver, mesher: mesher.NewMesher(resolver.Tables)}
}

func Open(options blockmesh.OpenOptions) (*Engine, error) {
	resolver, err := blockmesh.Open(options)
	if err != nil {
		return nil, err
	}
	return New(resolver), nil
}

func (engine *Engine) Build(javaHash uint32) (*Block, error) {
	state, ok := java.HashToBlock(javaHash)
	if !ok {
		return nil, fmt.Errorf("unknown Java block hash %d", javaHash)
	}
	mesh, err := engine.mesher.MeshBlock(javaHash)
	if err != nil {
		return nil, err
	}
	return &Block{State: state, Renderer: engine.Metadata.Renderers[state.NameWithNameSpace()], Mesh: mesh}, nil
}
