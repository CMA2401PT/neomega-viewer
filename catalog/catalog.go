// Package catalog enumerates state metadata, without expanding any geometry.
package catalog

import (
	"cmp"
	"fmt"
	"slices"

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

func Java() (*Catalog, error) {
	table, err := blockstate.Java()
	if err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	result := &Catalog{Groups: []Group{}, ByName: map[string][]blockstate.Binding{}, ByID: table.ByID}
	for _, binding := range table.ByID {
		name := string(binding.Java.Name)
		result.ByName[name] = append(result.ByName[name], binding)
	}
	for name, states := range result.ByName {
		slices.SortFunc(states, func(a, b blockstate.Binding) int { return cmp.Compare(a.ID, b.ID) })
		result.ByName[name] = states
		result.Groups = append(result.Groups, Group{Name: name, Count: len(states), ExampleID: states[0].ID, JavaNames: []string{name}})
	}
	slices.SortFunc(result.Groups, func(a, b Group) int { return cmp.Compare(a.Name, b.Name) })
	return result, nil
}
