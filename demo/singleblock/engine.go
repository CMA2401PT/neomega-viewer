// Package singleblock renders an isolated block without constructing a section.
package singleblock

import (
	"fmt"

	"neomega-blocks/ir"
	java "neomega-viewer-core/viewer/javastates"
	"neomega-viewer-core/viewer/cube"
	"neomega-viewer-core/viewer/mesher"
	"neomega-viewer-core/viewer/mesher/geometry"
	blockmesh "neomega-viewer-core/viewer/resources"
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
	return &Engine{Preset: resolver, mesher: mesher.NewMesher(cube.New(1), resolver.Tables)}
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
