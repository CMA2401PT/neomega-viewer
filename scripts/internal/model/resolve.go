package model

import (
	"fmt"
	"maps"
	"strings"
	"sync"

	"neomega-assets/asset"
)

type Resolved struct {
	Elements []asset.Element
	Textures map[string]asset.TextureBinding
}

// Resolver caches immutable inherited models. The supplied catalog must remain
// unchanged; returned definitions are read-only and can be shared concurrently.
type Resolver struct {
	catalog asset.Models
	cache   sync.Map
}

func NewResolver(catalog asset.Models) *Resolver { return &Resolver{catalog: catalog} }

func (resolver *Resolver) Resolve(id asset.ID) (*Resolved, error) {
	return resolver.resolve(id, map[asset.ID]bool{})
}

func (resolver *Resolver) resolve(id asset.ID, visiting map[asset.ID]bool) (*Resolved, error) {
	if cached, ok := resolver.cache.Load(id); ok {
		return cached.(*Resolved), nil
	}
	if visiting[id] {
		return nil, fmt.Errorf("model parent cycle at %s", id)
	}
	definition, ok := resolver.catalog[id.ExportName()]
	if !ok {
		return nil, fmt.Errorf("model %s not found", id)
	}
	visiting[id] = true
	defer delete(visiting, id)
	result := &Resolved{Textures: map[string]asset.TextureBinding{}}
	if definition.Parent != "" {
		parentID, err := asset.ParseID(string(definition.Parent))
		if err != nil {
			return nil, err
		}
		parent, err := resolver.resolve(parentID, visiting)
		if err != nil {
			return nil, err
		}
		result.Elements = parent.Elements
		result.Textures = maps.Clone(parent.Textures)
	}
	for name, binding := range definition.Textures {
		result.Textures[name] = binding
	}
	if definition.Elements != nil {
		result.Elements = *definition.Elements
	}
	resolver.cache.Store(id, result)
	return result, nil
}

// Texture follows slot aliases and retains the material hint along that chain.
func (resolved *Resolved) Texture(reference asset.TextureRef) (asset.ID, bool, error) {
	seen := map[string]bool{}
	translucent := false
	for strings.HasPrefix(string(reference), "#") {
		key := strings.TrimPrefix(string(reference), "#")
		if seen[key] {
			return "", false, fmt.Errorf("texture alias cycle at %s", key)
		}
		seen[key] = true
		binding, ok := resolved.Textures[key]
		if !ok {
			return "", false, fmt.Errorf("texture slot %s missing", key)
		}
		if binding.ForceTranslucent != nil {
			translucent = translucent || *binding.ForceTranslucent
		}
		reference = binding.Sprite
	}
	id, err := asset.ParseID(string(reference))
	return id, translucent, err
}
