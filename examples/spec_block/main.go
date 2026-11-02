package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"net/http"

	"celler/layer_storage"
	jmodel "neomega-blocks/geometry/java_block_model"
	"neomega-blocks/ir"
	preset "neomega-blocks/presets/java/v1/geometry"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer/dispatcher"
	"neomega-viewer/merger"
	"neomega-viewer/viewassets"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:2414", "HTTP listen address")
	flag.Parse()
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		log.Fatal(err)
	}
	// 陶罐 actor 按 NBT 选择纹样，需要与几何表配套的图集区域。
	atlasJSON, err := fs.ReadFile(viewassets.Resources(), "blocksAtlases.json")
	if err != nil {
		log.Fatal(err)
	}
	var atlas jmodel.Atlas
	if err := json.Unmarshal(atlasJSON, &atlas); err != nil {
		log.Fatal(err)
	}
	pipeline := dispatcher.NewJavaPipeline(resources.Tables, &atlas, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/subchunk", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("subchunk") != "default" {
			http.Error(w, "unknown subchunk", http.StatusNotFound)
			return
		}
		// 在请求内分发静态网格、effect 和 block actor，再编码为完整快照。
		result, err := pipeline.Build(
			defaultSubchunk(),
			dispatcher.Neighbours{},
			dispatcher.Options{},
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, diagnostic := range result.Diagnostics {
			log.Printf("%s at %v: %s (%v)", diagnostic.Kind, diagnostic.Pos, diagnostic.State, diagnostic.Err)
		}
		var data bytes.Buffer
		if err := result.Frame.Marshal(&data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", merger.ContentType)
		if _, err := w.Write(data.Bytes()); err != nil {
			log.Print(err)
		}
	})
	mux.Handle("GET /textures/", http.StripPrefix("/textures/", http.FileServerFS(viewassets.Resources())))
	mux.Handle("GET /", http.FileServer(http.Dir("dist")))
	log.Printf("http://%s", *listen)
	log.Fatal(http.ListenAndServe(*listen, mux))
}

// 一个 16×16×16 子区块：木屋外放三个附魔台、三个不同朝向的纹样陶罐。
func defaultSubchunk() dispatcher.Section {
	air := ir.IRBlock("minecraft:air []")
	stone := ir.IRBlock("minecraft:stone []")
	oak := ir.IRBlock("minecraft:oak_planks []")
	spruce := ir.IRBlock("minecraft:spruce_planks []")
	glass := ir.IRBlock("minecraft:glass []")
	section := dispatcher.Section{
		Blocks:   layer_storage.MakePaletteStorage(air),
		Entities: map[dispatcher.CellPos]nbt.Compound{},
	}
	set := func(x, y, z int, state ir.IRBlock) {
		section.Blocks.Set(layer_storage.At(x, y, z), state)
	}
	for x := 2; x < 14; x++ {
		for z := 2; z < 14; z++ {
			set(x, 0, z, stone)
		}
	}
	for x := 5; x <= 10; x++ {
		for z := 5; z <= 10; z++ {
			set(x, 1, z, oak)
			for y := 2; y <= 5; y++ {
				if x == 5 || x == 10 || z == 5 || z == 10 {
					set(x, y, z, oak)
				}
			}
		}
	}
	// 朝 +Z 开门洞；两侧和后墙留窗。
	for x := 7; x <= 8; x++ {
		set(x, 2, 10, air)
		set(x, 3, 10, air)
		set(x, 3, 5, glass)
	}
	for z := 7; z <= 8; z++ {
		set(5, 3, z, glass)
		set(10, 3, z, glass)
	}
	for y := 6; y <= 9; y++ {
		inset := y - 6
		for x := 4 + inset; x <= 11-inset; x++ {
			for z := 4; z <= 11; z++ {
				set(x, y, z, spruce)
			}
		}
	}
	for _, x := range []int{4, 7, 10} {
		set(x, 1, 12, ir.IRBlock("minecraft:enchanting_table []"))
	}
	for i, facing := range []string{"north", "east", "west"} {
		pos := dispatcher.CellPos{12, 1, uint8(4 + 4*i)}
		state := ir.IRBlock(`minecraft:decorated_pot ["facing"="` + facing + `","cracked"="false","waterlogged"="false"]`).Tidy()
		set(int(pos[0]), int(pos[1]), int(pos[2]), state)
		// 四面顺序：back、left、right、front；纹样来自 NBT，而非方块状态。
		section.Entities[pos] = nbt.Compound{
			"sherds": []string{
				"minecraft:angler_pottery_sherd",
				"minecraft:archer_pottery_sherd",
				"minecraft:blade_pottery_sherd",
				"minecraft:arms_up_pottery_sherd",
			},
		}
	}
	return section
}
