// Package singleblock renders an isolated block without constructing a section.
package singleblock

import (
	"neomega-assets/asset"
	"neomega-viewer/blockmesh"
	"neomega-viewer/blockstate"
	"neomega-viewer/geometry"
)

type OpenOptions = blockmesh.OpenOptions
type Options = blockmesh.Options
type Renderer = blockmesh.Renderer

type Engine struct{ *blockmesh.Resolver }
type Block struct{ *blockmesh.Block }

type Packet struct {
	Binding      blockstate.Binding `json:"binding"`
	State        blockstate.State   `json:"state"`
	Renderer     Renderer           `json:"renderer"`
	Applications []asset.Apply      `json:"applications"`
	Geometry     geometry.Buffers   `json:"geometry"`
}

func (block *Block) Packet() Packet {
	return Packet{Binding: block.Binding, State: block.State, Renderer: block.Renderer, Applications: block.Applications, Geometry: block.Mesh.Buffers()}
}

func New(resolver *blockmesh.Resolver) *Engine { return &Engine{Resolver: resolver} }

func Open(options OpenOptions) (*Engine, error) {
	resolver, err := blockmesh.Open(options)
	if err != nil {
		return nil, err
	}
	return New(resolver), nil
}

func (engine *Engine) Build(id uint32, options Options) (*Block, error) {
	block, err := engine.Resolver.Resolve(id, options)
	if err != nil {
		return nil, err
	}
	return &Block{Block: block}, nil
}
