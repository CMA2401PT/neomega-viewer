package previewitems_test

import (
	"reflect"
	"testing"

	"neomega-converter-core/be2je/items"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer-core/viewer/entity_render"
	"neomega-viewer-core/viewer/geometry"
	"neomega-viewer-core/viewer/item_render"
	preset "neomega-viewer-core/viewer/resources"
	"neomega-viewer/internal/previewitems"
)

func TestBedrockItemsReachFrameGeometry(t *testing.T) {
	assets, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	resolver := previewitems.Resolver(assets.Items)
	renderer := entity_render.New(assets.Items, item_render.New(assets.Items))
	for _, name := range []string{"poplar_planks", "poplar_hanging_sign", "copper_sword", "copper_bars", "iron_chain", "white_harness", "sulfur_spike", "oak_hanging_sign", "beetroot", "cocoa_beans", "big_dripleaf"} {
		t.Run(name, func(t *testing.T) {
			source := nbt.Compound{"Name": "minecraft:" + name, "Count": int8(1), "Damage": int16(0), "tag": nbt.Compound{"display": nbt.Compound{"Name": "展示用"}}}
			before, err := nbt.EncodeToNBTBytes(source, nbt.OptEncodeToNBT{Encoding: nbt.EncodingLittleEndian})
			if err != nil {
				t.Fatal(err)
			}
			item := items.ConvertItemWithResolver(source, func(name string) { t.Errorf("unexpected conversion failure: %s", name) }, resolver)
			if item == nil || item["id"] != "minecraft:"+name {
				t.Fatalf("item: %+v", item)
			}
			if item["components"].(nbt.Compound)["minecraft:custom_name"] == nil {
				t.Fatal("lost metadata")
			}
			frame := entity_render.Entity{ID: "minecraft:item_frame", NBT: nbt.Compound{"Facing": int8(3), "Item": item}}
			mesh, issues := renderer.Build(entity_render.Input{List: []entity_render.Entity{frame}})
			if len(issues) != 0 || len(mesh.Quads) <= len(assets.Items.Models["minecraft:block/item_frame"].Quads) {
				t.Fatalf("frame lost item: %+v", issues)
			}
			var original nbt.Compound
			if err := nbt.DecodeFromNBTBytes(before, &original, nbt.OptDecodeFromNBT{Encoding: nbt.EncodingLittleEndian}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(original, source) {
				t.Fatal("source mutated")
			}
		})
	}
}

func TestResolverKeepsVariantsAndUnknowns(t *testing.T) {
	catalog := &item_render.Catalog{Items: map[string]geometry.Mesh{"minecraft:stone": {}, "minecraft:granite": {}, "minecraft:new_item": {}}, Unsupported: map[string]string{"minecraft:shield": "special"}}
	resolver := previewitems.Resolver(catalog)
	for _, test := range []struct {
		name, id, want string
		aux            any
	}{
		{"minecraft:stone", "minecraft:granite", "minecraft:granite", int16(1)},
		{"minecraft:new_item", "", "", int16(1)},
		{"minecraft:new_item", "", "", "0"},
		{"example:unknown", "", "", int16(0)},
		{"minecraft:shield", "", "minecraft:shield", int16(0)},
	} {
		id, _ := resolver(nbt.Compound{"Name": test.name, "Damage": test.aux}, test.id, false)
		if id != test.want {
			t.Fatalf("%+v -> %s", test, id)
		}
	}
}
