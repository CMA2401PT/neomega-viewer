package blockmesh

import (
	"sync"

	"neomega-assets/asset"
	"neomega-viewer/blockstate"
	"neomega-viewer/resources"
)

type OpenOptions struct {
	// 留空使用随库分发的表，外部资源包可指定配套表文件。
	TablesPath string
}

type Resolver struct {
	identity *blockstate.Table
	tables   *Tables
}

var bundledTables = sync.OnceValues(func() (*Tables, error) {
	// 多个渲染入口共享同一份解码结果。
	file, err := resources.Files.Open("geometry.gob")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return ReadTables(file)
})

func Open(options OpenOptions) (*Resolver, error) {
	var tables *Tables
	var err error
	if options.TablesPath == "" {
		tables, err = bundledTables()
	} else {
		tables, err = LoadTables(options.TablesPath)
	}
	if err != nil {
		return nil, err
	}
	identity, err := blockstate.Java()
	if err != nil {
		return nil, err
	}
	return &Resolver{identity: identity, tables: tables}, nil
}

func (engine *Resolver) Tables() (*Tables, error) {
	return engine.tables, nil
}

func (engine *Resolver) Version() string {
	return engine.tables.Version
}

func (engine *Resolver) Renderer(name asset.ID) Renderer {
	return engine.tables.Renderers[name]
}

// Binding resolves identity only, without selecting or baking models.
func (engine *Resolver) Binding(id uint32) (blockstate.Binding, error) {
	return engine.identity.Resolve(id)
}
