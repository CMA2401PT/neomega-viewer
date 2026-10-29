// Package singleblock renders an isolated block without constructing a section.
package singleblock

import (
	"neomega-viewer/blockstate"
	"neomega-viewer/demo/blockmesh"
	"neomega-viewer/geometry"
	"neomega-viewer/mesher"
)

type OpenOptions = blockmesh.OpenOptions

type Engine struct {
	*blockmesh.Resolver
	mesher *mesher.Mesher[uint32]
}

type Renderer = blockstate.Renderer

type Block struct {
	Binding  blockstate.Binding
	Renderer Renderer
	Mesh     geometry.Mesh
}

type Packet struct {
	Binding  blockstate.Binding `json:"binding"`
	Renderer Renderer           `json:"renderer"`
	Geometry geometry.Buffers   `json:"geometry"`
}

func (block *Block) Packet() Packet {
	return Packet{
		Binding:  block.Binding,
		Renderer: block.Renderer,
		Geometry: block.Mesh.Buffers(),
	}
}

func New(resolver *blockmesh.Resolver) *Engine {
	return &Engine{Resolver: resolver, mesher: mesher.NewMesher(resolver.Tables())}
}

func Open(options OpenOptions) (*Engine, error) {
	resolver, err := blockmesh.Open(options)
	if err != nil {
		return nil, err
	}
	return New(resolver), nil
}

func (engine *Engine) Build(javaHash uint32) (*Block, error) {
	mesh, err := engine.mesher.MeshBlock(javaHash)
	if err != nil {
		return nil, err
	}
	binding, err := engine.Binding(javaHash)
	if err != nil {
		return nil, err
	}
	profile := engine.Renderer(binding.Java.Name)
	return &Block{
		Binding:  binding,
		Renderer: Renderer{Kind: profile.Kind, Family: profile.Family},
		Mesh:     mesh,
	}, nil
}
