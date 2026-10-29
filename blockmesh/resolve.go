// Package blockmesh resolves one stored block into local-space static geometry.
// Single-block and subchunk entry points share this parser, not their scene logic.
// 解释与烘焙仅在建表时执行，渲染入口共享生成结果。
package blockmesh

import (
	"fmt"
	"sync"

	assetparse "neomega-assets/parse"
	blocks "neomega-blocks/presets/block-converts/v1"
	"neomega-viewer/blockstate"
	"neomega-viewer/material"
	"neomega-viewer/model"
	"neomega-viewer/resources"
)

type OpenOptions struct {
	AssetsDir string
	Policy    material.Policy
}

type Resolver struct {
	Assets    *assetparse.Assets
	identity  *blockstate.Table
	models    *model.Resolver
	materials *material.Library
	tables    func() (*Tables, error)
}

func Open(options OpenOptions) (*Resolver, error) {
	assets, err := assetparse.Load(options.AssetsDir)
	if err != nil {
		return nil, err
	}
	if assets.BuildInfo.Version != blocks.JavaTargetVersion {
		return nil, fmt.Errorf("resources are Java %s; Java states target %s", assets.BuildInfo.Version, blocks.JavaTargetVersion)
	}
	identity, err := blockstate.Java()
	if err != nil {
		return nil, err
	}
	if options.Policy == nil {
		options.Policy = resources.PreviewPolicy{}
	}
	engine := &Resolver{
		Assets:    assets,
		identity:  identity,
		models:    model.NewResolver(assets.Models),
		materials: material.New(assets.Atlas, assets.AtlasImage, options.Policy),
	}
	engine.tables = sync.OnceValues(engine.buildTables)
	return engine, nil
}

// Binding resolves identity only, without selecting or baking models.
func (engine *Resolver) Binding(id uint32) (blockstate.Binding, error) {
	return engine.identity.Resolve(id)
}
