// Package catalog groups canonical Java IR blocks for browsing, without loading geometry.
package catalog

import (
	"cmp"
	"slices"

	"neomega-blocks/ir"
	java "neomega-viewer-core/viewer/javastates"
)

type Group struct {
	Name      string   `json:"name"`
	Count     int      `json:"count"`
	ExampleID uint32   `json:"exampleId"`
	JavaNames []string `json:"javaNames"`
}

type Catalog struct {
	Groups []Group
	ByName map[string][]ir.IRBlock
}

func Java() *Catalog {
	result := &Catalog{ByName: map[string][]ir.IRBlock{}}
	for block := range java.JavaFullStates() {
		name := block.NameWithNameSpace()
		result.ByName[name] = append(result.ByName[name], block)
	}
	for name, states := range result.ByName {
		slices.SortFunc(states, func(a, b ir.IRBlock) int { return cmp.Compare(a.Hash(), b.Hash()) })
		result.Groups = append(result.Groups, Group{Name: name, Count: len(states), ExampleID: states[0].Hash(), JavaNames: []string{name}})
	}
	slices.SortFunc(result.Groups, func(a, b Group) int { return cmp.Compare(a.Name, b.Name) })
	return result
}
