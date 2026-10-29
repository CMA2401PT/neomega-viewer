// Package singleblock renders an isolated block without constructing a section.
package singleblock

import (
	"neomega-assets/asset"
	"neomega-viewer/blockmesh"
	"neomega-viewer/blockstate"
	"neomega-viewer/geometry"
)

type OpenOptions = blockmesh.OpenOptions

type Engine struct{ *blockmesh.Resolver }

type Renderer struct {
	Kind   asset.RendererKind `json:"kind"`
	Family string             `json:"family,omitempty"`
}

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

func New(resolver *blockmesh.Resolver) *Engine { return &Engine{Resolver: resolver} }

func Open(options OpenOptions) (*Engine, error) {
	resolver, err := blockmesh.Open(options)
	if err != nil {
		return nil, err
	}
	return New(resolver), nil
}

func (engine *Engine) Build(javaHash uint32) (*Block, error) {
	tables, err := engine.Tables()
	if err != nil {
		return nil, err
	}
	ids, err := tables.ModelIDs(javaHash)
	if err != nil {
		return nil, err
	}
	mesh := geometry.Mesh{Quads: []geometry.Quad{}}
	for _, id := range ids {
		shape, err := tables.Shape(id)
		if err != nil {
			return nil, err
		}
		// 输出独立网格，调用者修改它不会改变共享表。
		mesh.Quads = append(mesh.Quads, shape.Quads...)
	}
	binding, err := engine.Binding(javaHash)
	if err != nil {
		return nil, err
	}
	profile := engine.Assets.Renderers.Blocks[binding.Java.Name]
	return &Block{
		Binding:  binding,
		Renderer: Renderer{Kind: profile.Kind, Family: profile.Family},
		Mesh:     mesh,
	}, nil
}
