// Package previewitems aligns displayed Bedrock items with the loaded preview catalog.
package previewitems

import (
	"strings"

	"neomega-converter-core/be2je/items"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer-core/viewer/item_render"
)

// Resolver keeps recognized legacy variants, but recovers same-name items when
// the old export table has no item or mistakenly returns a placed-block ID.
// Unsupported catalog entries are recognized too, so rendering can report the
// actual unsupported appearance instead of conversion silently losing the item.
func Resolver(catalog *item_render.Catalog) items.IDResolver {
	known := func(id string) bool {
		_, model := catalog.Items[id]
		_, unsupported := catalog.Unsupported[id]
		return model || unsupported
	}
	return func(source nbt.Compound, id string, blockItem bool) (string, bool) {
		if known(id) {
			return id, blockItem
		}
		// Nonzero legacy data values may select a different item variant. Do not
		// replace them with an unverified same-name default.
		switch value := source["Damage"].(type) {
		case nil:
		case int8:
			if value != 0 {
				return id, blockItem
			}
		case int16:
			if value != 0 {
				return id, blockItem
			}
		case int32:
			if value != 0 {
				return id, blockItem
			}
		default:
			return id, blockItem
		}
		name, _ := source["Name"].(string)
		if name != "" && !strings.Contains(name, ":") {
			name = "minecraft:" + name
		}
		if known(name) {
			_, hasBlock := source["Block"].(nbt.Compound)
			return name, blockItem || hasBlock
		}
		return id, blockItem
	}
}
