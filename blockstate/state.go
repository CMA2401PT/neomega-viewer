// Package blockstate translates a stored block ID into a Java state.
// IDs in the built-in adapter are NEMC Bedrock network hashes, not list indices.
package blockstate

import (
	"fmt"
	"maps"

	"neomega-assets/asset"
	"neomega-blocks/ir"
	blocks "neomega-blocks/presets/block-converts/v1"
)

type State struct {
	Name       asset.ID          `json:"name"`
	Properties map[string]string `json:"properties"`
}

type Binding struct {
	ID      uint32     `json:"id"`
	Bedrock ir.IRBlock `json:"bedrock"`
	Java    State      `json:"java"`
}

// Resolver lets a caller provide another uint32 ID space without changing model
// selection or geometry generation.
type Resolver interface {
	Resolve(id uint32) (Binding, error)
}

type NEMC struct{}

func (NEMC) Resolve(id uint32) (Binding, error) {
	bedrock, ok := blocks.BedrockHashToBlockStrict(id)
	if !ok {
		return Binding{}, fmt.Errorf("unknown NEMC network hash %d", id)
	}
	java, ok := blocks.ConvertToJava(bedrock)
	if !ok {
		return Binding{}, fmt.Errorf("no Java conversion for %s", bedrock)
	}
	name, err := asset.ParseID(java.NameWithNameSpace())
	if err != nil {
		return Binding{}, err
	}
	properties := map[string]string{}
	for key, value := range java.States().KeyValues {
		properties[key] = fmt.Sprint(value)
	}
	return Binding{ID: id, Bedrock: bedrock, Java: State{Name: name, Properties: properties}}, nil
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
