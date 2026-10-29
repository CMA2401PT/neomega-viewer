// Package blockmesh resolves one stored block into local-space static geometry.
// Single-block and subchunk entry points share this parser, not their scene logic.
package blockmesh

import (
	"fmt"
	"sync"

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

type Resolver struct {
	Assets    *assetparse.Assets
	identity  blockstate.Resolver
	models    *model.Resolver
	materials *material.Library
	tables    func() (*Tables, error)
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

func Open(options OpenOptions) (*Resolver, error) {
	assets, err := assetparse.Load(options.AssetsDir)
	if err != nil {
		return nil, err
	}
	if options.Resolver == nil {
		if assets.BuildInfo.Version != blocks.JavaTargetVersion {
			return nil, fmt.Errorf("resources are Java %s; Java states target %s", assets.BuildInfo.Version, blocks.JavaTargetVersion)
		}
		options.Resolver, err = blockstate.Java()
		if err != nil {
			return nil, err
		}
	}
	if options.Policy == nil {
		options.Policy = resources.PreviewPolicy{}
	}
	engine := &Resolver{
		Assets:    assets,
		identity:  options.Resolver,
		models:    model.NewResolver(assets.Models),
		materials: material.New(assets.Atlas, assets.AtlasImage, options.Policy),
	}
	engine.tables = sync.OnceValues(engine.buildTables)
	return engine, nil
}

func (engine *Resolver) Resolve(id uint32, options Options) (*Block, error) {
	binding, err := engine.identity.Resolve(id)
	if err != nil {
		return nil, err
	}
	return engine.Bake(binding, options)
}

// Binding resolves identity only, without selecting or baking models.
func (engine *Resolver) Binding(id uint32) (blockstate.Binding, error) {
	return engine.identity.Resolve(id)
}

// Bake shares model parsing after a caller has obtained an immutable binding.
func (engine *Resolver) Bake(binding blockstate.Binding, options Options) (*Block, error) {
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
