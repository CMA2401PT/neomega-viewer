package blockmesh

import (
	"encoding/json"
	"os"
	"sync"

	"neomega-assets/asset"
	"neomega-viewer/blockstate"
	"neomega-viewer/mesher"
	"neomega-viewer/resources"
)

type OpenOptions struct {
	// 留空使用随库分发的表，外部表的 Java 元数据来自同名 .java.json 文件。
	TablesPath string
}

type Resolver struct {
	identity *blockstate.Table
	tables   *mesher.Tables[uint32]
	metadata *blockstate.Metadata
}

var bundledTables = sync.OnceValues(func() (*mesher.Tables[uint32], error) {
	// 多个演示入口共享同一份解码结果。
	file, err := resources.Files.Open("geometry.nvtables")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return mesher.DecodeTables[uint32](file)
})

func Open(options OpenOptions) (*Resolver, error) {
	var tables *mesher.Tables[uint32]
	var metadata blockstate.Metadata
	var err error
	if options.TablesPath == "" {
		tables, err = bundledTables()
	} else {
		file, openErr := os.Open(options.TablesPath)
		if openErr != nil {
			return nil, openErr
		}
		defer file.Close()
		tables, err = mesher.DecodeTables[uint32](file)
	}
	if err != nil {
		return nil, err
	}
	var data []byte
	if options.TablesPath == "" {
		data, err = resources.Files.ReadFile("geometry.nvtables.java.json")
	} else {
		data, err = os.ReadFile(options.TablesPath + ".java.json")
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}
	identity, err := blockstate.Java()
	if err != nil {
		return nil, err
	}
	return &Resolver{identity: identity, tables: tables, metadata: &metadata}, nil
}

func (engine *Resolver) Tables() *mesher.Tables[uint32] {
	return engine.tables
}

func (engine *Resolver) Metadata() *blockstate.Metadata {
	return engine.metadata
}

func (engine *Resolver) Version() string {
	return engine.metadata.Version
}

func (engine *Resolver) Renderer(name asset.ID) blockstate.Renderer {
	return engine.metadata.Renderers[name]
}

// Binding resolves identity only, without selecting or baking models.
func (engine *Resolver) Binding(id uint32) (blockstate.Binding, error) {
	return engine.identity.Resolve(id)
}
