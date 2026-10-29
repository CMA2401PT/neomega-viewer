// Package blockstate translates a stored block ID into a Java state.
// IDs in the built-in table are hashes of canonical Java states, not list indices.
package blockstate

import (
	"fmt"
	"maps"
	"sync"

	"neomega-assets/asset"
	blocks "neomega-blocks/presets/block-converts/v1"
)

type State struct {
	Name       asset.ID          `json:"name"`
	Properties map[string]string `json:"properties"`
}

type Binding struct {
	ID   uint32 `json:"id"`
	Java State  `json:"java"`
}

// Resolver looks up canonical Java block hashes independently of model selection
// and geometry generation.
type Resolver interface {
	Resolve(id uint32) (Binding, error)
}

type Table struct {
	ByID map[uint32]Binding
}

var javaTable = sync.OnceValues(func() (*Table, error) {
	// 目录与渲染共享状态全集，避免两条入口使用不同的 ID 来源。
	result := &Table{ByID: map[uint32]Binding{}}
	for java := range blocks.JavaFullStates() {
		name, err := asset.ParseID(java.NameWithNameSpace())
		if err != nil {
			return nil, err
		}
		properties := map[string]string{}
		for key, value := range java.States().KeyValues {
			properties[key] = fmt.Sprint(value)
		}
		// JavaFullStates 已 Tidy，属性顺序与上游的规范哈希一致。
		id := java.Hash()
		result.ByID[id] = Binding{ID: id, Java: State{Name: name, Properties: properties}}
	}
	return result, nil
})

func Java() (*Table, error) {
	return javaTable()
}

func (table *Table) Resolve(id uint32) (Binding, error) {
	binding, ok := table.ByID[id]
	if !ok {
		return Binding{}, fmt.Errorf("unknown Java block hash %d", id)
	}
	return binding, nil
}

func (state State) With(overrides map[string]string) State {
	properties := make(map[string]string, len(state.Properties)+len(overrides))
	maps.Copy(properties, state.Properties)
	state.Properties = properties
	for key, value := range overrides {
		state.Properties[key] = value
	}
	return state
}
