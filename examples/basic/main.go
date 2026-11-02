package main

import (
	"bytes"
	"flag"
	"log"
	"net/http"

	"neomega-blocks/ir"
	preset "neomega-blocks/presets/java/v1/geometry"
	"neomega-viewer/mesher"
	"neomega-viewer/mesher/geometry"
	"neomega-viewer/viewassets"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:2413", "HTTP listen address")
	flag.Parse()
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		log.Fatal(err)
	}
	meshBuilder := mesher.NewMesher(resources.Tables)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/subchunk", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("subchunk") != "default" {
			http.Error(w, "unknown subchunk", http.StatusNotFound)
			return
		}
		// 每个请求直接构造方块、剔面、合并，再编码成 NVMS 网格。
		result, err := meshBuilder.MeshSubchunk(defaultSubchunk(), mesher.Options{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var data bytes.Buffer
		if err := result.Mesh.Marshal(&data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", geometry.ContentType)
		if _, err := w.Write(data.Bytes()); err != nil {
			log.Print(err)
		}
	})
	mux.Handle("GET /textures/", http.StripPrefix("/textures/", http.FileServerFS(viewassets.Resources())))
	mux.Handle("GET /", http.FileServer(http.Dir("dist")))
	log.Printf("http://%s", *listen)
	log.Fatal(http.ListenAndServe(*listen, mux))
}

// 一个 16×16×16 子区块：石头平台、木屋、玻璃窗与阶梯形屋顶。
func defaultSubchunk() []uint32 {
	air := ir.IRBlock("minecraft:air []").Hash()
	stone := ir.IRBlock("minecraft:stone []").Hash()
	oak := ir.IRBlock("minecraft:oak_planks []").Hash()
	spruce := ir.IRBlock("minecraft:spruce_planks []").Hash()
	glass := ir.IRBlock("minecraft:glass []").Hash()
	blocks := make([]uint32, 16*16*16)
	for i := range blocks {
		blocks[i] = air
	}
	set := func(x, y, z int, id uint32) {
		// MeshSubchunk 的布局：Y 最快，Z 次之，X 最慢。
		blocks[x<<8|z<<4|y] = id
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
	return blocks
}
