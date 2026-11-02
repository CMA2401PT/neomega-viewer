package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"strconv"

	jmodel "neomega-blocks/geometry/java_block_model"
	preset "neomega-blocks/presets/java/v1/geometry"
	"neomega-viewer/dispatcher"
	"neomega-viewer/merger"
	"neomega-viewer/viewassets"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:2416", "HTTP 监听地址")
	flag.Parse()
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		log.Fatal(err)
	}
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
	mux.HandleFunc("GET /api/section", func(w http.ResponseWriter, r *http.Request) {
		var pos dispatcher.SectionPos
		for name, target := range map[string]*int32{"x": &pos.X, "z": &pos.Z} {
			value, err := strconv.ParseInt(r.URL.Query().Get(name), 10, 32)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			*target = int32(value)
		}
		// 同时读取六向邻居，让相邻子区块边界也能正确剔面。
		results, err := pipeline.BuildSections(
			r.Context(),
			generatedWorld{},
			[]dispatcher.SectionPos{pos},
			dispatcher.Options{},
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		result := results[0].Result
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
