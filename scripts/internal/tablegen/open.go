package tablegen

import (
	"fmt"

	assetparse "neomega-assets/parse"
	blocks "neomega-blocks/presets/block-converts/v1"
	"neomega-viewer/scripts/internal/materials"
	"neomega-viewer/scripts/internal/model"
	"neomega-viewer/scripts/internal/resources"
)

type OpenOptions struct {
	AssetsDir string
	Policy    materials.Policy
}

type Builder struct {
	Assets    *assetparse.Assets
	models    *model.Resolver
	materials *materials.Library
}

func Open(options OpenOptions) (*Builder, error) {
	assets, err := assetparse.Load(options.AssetsDir)
	if err != nil {
		return nil, err
	}
	if assets.BuildInfo.Version != blocks.JavaTargetVersion {
		return nil, fmt.Errorf("resources are Java %s; Java states target %s", assets.BuildInfo.Version, blocks.JavaTargetVersion)
	}
	if options.Policy == nil {
		options.Policy = resources.PreviewPolicy{}
	}
	engine := &Builder{
		Assets:    assets,
		models:    model.NewResolver(assets.Models),
		materials: materials.New(assets.Atlas, assets.AtlasImage, options.Policy),
	}
	return engine, nil
}
