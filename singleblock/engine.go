// Package singleblock composes ID conversion, model selection and face baking.
// It is an independent rendering entry point, with no world or section object.
package singleblock

import (
	"fmt"

	"neomega-assets/asset"
	assetparse "neomega-assets/parse"
	blocks "neomega-blocks/presets/block-converts/v1"
	"neomega-viewer/blockstate"
	"neomega-viewer/geometry"
	"neomega-viewer/material"
	"neomega-viewer/model"
	"neomega-viewer/resources"
)

type OpenOptions struct {
	AssetsDir string
	Resolver  blockstate.Resolver
	Policy    material.Policy
}

type Engine struct {
	Assets    *assetparse.Assets
	identity  blockstate.Resolver
	models    *model.Resolver
	materials *material.Library
}

type Options struct {
	Seed uint64 `json:"seed"`
	// Properties describe a static Java preview, e.g. north=true for a fence.
	// They do not change the original ID-to-state mapping in Binding.
	Properties map[string]string `json:"properties,omitempty"`
}

// Renderer is a static description, independent of runtime fields in the source
// resource table. Geometry is always determined by state and preview options.
type Renderer struct {
	Kind   asset.RendererKind `json:"kind"`
	Family string             `json:"family,omitempty"`
}

type Block struct {
	Binding      blockstate.Binding
	State        blockstate.State
	Renderer     Renderer
	Applications []asset.Apply
	Mesh         geometry.Mesh
}

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

func Open(options OpenOptions) (*Engine, error) {
	assets, err := assetparse.Load(options.AssetsDir)
	if err != nil {
		return nil, err
	}
	if options.Resolver == nil {
		if assets.BuildInfo.Version != blocks.JavaTargetVersion {
			return nil, fmt.Errorf("resources are Java %s; NEMC converter targets %s", assets.BuildInfo.Version, blocks.JavaTargetVersion)
		}
		options.Resolver = blockstate.NEMC{}
	}
	if options.Policy == nil {
		options.Policy = resources.PreviewPolicy{}
	}
	return &Engine{
		Assets:    assets,
		identity:  options.Resolver,
		models:    model.NewResolver(assets.Models),
		materials: material.New(assets.Atlas, assets.AtlasImage, options.Policy),
	}, nil
}

func (engine *Engine) Build(id uint32, options Options) (*Block, error) {
	binding, err := engine.identity.Resolve(id)
	if err != nil {
		return nil, err
	}
	state := binding.Java
	definition, hasDefinition := engine.Assets.BlockState(state.Name)
	// Complete only a display condition the actual model asks for. Most solid
	// blocks cannot be waterlogged and should not expose a fictitious control.
	if _, needed := model.PropertyValues(definition, nil)["waterlogged"]; needed {
		state = resources.PreviewProperties(state)
	}
	state = state.With(options.Properties)
	profile, ok := engine.Assets.Renderers.Blocks[state.Name]
	if !ok {
		return nil, fmt.Errorf("no renderer entry for %s", state.Name)
	}
	result := &Block{Binding: binding, State: state, Renderer: Renderer{Kind: profile.Kind, Family: profile.Family}, Applications: []asset.Apply{}, Mesh: geometry.Mesh{Quads: []geometry.Quad{}}}
	switch profile.Kind {
	case asset.RendererInvisible:
		return result, nil
	case asset.RendererDynamic:
		return nil, fmt.Errorf("%s has no fixed static shape", state.Name)
	case asset.RendererFluid:
		mesh, err := geometry.Bake(resources.FluidPreview(state, profile.Family), asset.Apply{}, state, engine.materials)
		result.Mesh = mesh
		return result, err
	}
	if !hasDefinition {
		return nil, fmt.Errorf("blockstate %s missing", state.Name)
	}
	applications, err := model.Select(definition, state.Properties, options.Seed)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", state.Name, err)
	}
	result.Applications = applications
	for _, application := range applications {
		id, err := asset.ParseID(string(application.Model))
		if err != nil {
			return nil, err
		}
		resolved, err := engine.models.Resolve(id)
		if err != nil {
			return nil, err
		}
		mesh, err := geometry.Bake(resolved, application, state, engine.materials)
		if err != nil {
			return nil, fmt.Errorf("model %s: %w", id, err)
		}
		result.Mesh.Quads = append(result.Mesh.Quads, mesh.Quads...)
	}
	return result, nil
}
