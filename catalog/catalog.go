// Package catalog enumerates state metadata, without expanding any geometry.
package catalog

import (
	"cmp"
	"fmt"
	"slices"

	blocks "neomega-blocks/presets/block-converts/v1"
	"neomega-viewer/blockstate"
)

type Group struct {
	Name      string   `json:"name"`
	Count     int      `json:"count"`
	ExampleID uint32   `json:"exampleId"`
	JavaNames []string `json:"javaNames"`
}

type Catalog struct {
	Groups []Group
	ByName map[string][]blockstate.Binding
	ByID   map[uint32]blockstate.Binding
}

func NEMC() (*Catalog, error) {
	result := &Catalog{Groups: []Group{}, ByName: map[string][]blockstate.Binding{}, ByID: map[uint32]blockstate.Binding{}}
	resolver := blockstate.NEMC{}
	for id, bedrock := range blocks.BedrockBlocks() {
		binding, err := resolver.Resolve(id)
		if err != nil {
			return nil, fmt.Errorf("catalog: %w", err)
		}
		result.ByID[id] = binding
		name := bedrock.NameWithNameSpace()
		result.ByName[name] = append(result.ByName[name], binding)
	}
	for name, states := range result.ByName {
		slices.SortFunc(states, func(a, b blockstate.Binding) int { return cmp.Compare(string(a.Bedrock), string(b.Bedrock)) })
		javaNames := []string{}
		for _, state := range states {
			javaNames = append(javaNames, string(state.Java.Name))
		}
		slices.Sort(javaNames)
		javaNames = slices.Compact(javaNames)
		result.ByName[name] = states
		result.Groups = append(result.Groups, Group{Name: name, Count: len(states), ExampleID: states[0].ID, JavaNames: javaNames})
	}
	slices.SortFunc(result.Groups, func(a, b Group) int { return cmp.Compare(a.Name, b.Name) })
	return result, nil
}
