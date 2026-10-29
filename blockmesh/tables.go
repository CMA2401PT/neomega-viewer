package blockmesh

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"

	"neomega-assets/asset"
	"neomega-viewer/blockstate"
	"neomega-viewer/geometry"
)

type ModelID uint32

type SkippedState struct {
	Binding blockstate.Binding
	Reason  string
}

type Renderer struct {
	Kind   asset.RendererKind `json:"kind"`
	Family string             `json:"family,omitempty"`
}

type Tables struct {
	// 表和查询返回的切片共享且只读，供多个渲染入口复用。
	Index  map[uint32][]ModelID
	Shapes []geometry.Shape
	// 仅流体几何有此映射，上方同类流体出现时改用满高度几何。
	FullHeight map[ModelID]ModelID
	// 响应元数据随几何保存，运行时无需再读取模型资源。
	Version   string
	Renderers map[asset.ID]Renderer
	Skipped   []SkippedState
}

func ReadTables(reader io.Reader) (*Tables, error) {
	var tables Tables
	if err := gob.NewDecoder(reader).Decode(&tables); err != nil {
		return nil, err
	}
	return &tables, nil
}

func LoadTables(path string) (*Tables, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return ReadTables(file)
}

func (tables *Tables) ModelIDs(javaHash uint32) ([]ModelID, error) {
	ids, ok := tables.Index[javaHash]
	if !ok {
		return nil, fmt.Errorf("no static geometry for Java block hash %d", javaHash)
	}
	return ids, nil
}

func (tables *Tables) Shape(id ModelID) (*geometry.Shape, error) {
	if uint64(id) >= uint64(len(tables.Shapes)) {
		return nil, fmt.Errorf("unknown model ID %d", id)
	}
	return &tables.Shapes[id], nil
}
