package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	jmodel "neomega-blocks/geometry/java_block_model"
	blockmesh "neomega-blocks/presets/java/v1/geometry"
	"neomega-converter/fixer"
	"neomega-viewer/demo/catalog"
	"neomega-viewer/demo/httpapi"
	"neomega-viewer/demo/singleblock"
	"neomega-viewer/mesher"
)

func main() {
	assets := flag.String("assets", "../neomega-assets/dist/final", "matching resource bundle")
	tables := flag.String("tables", "", "geometry table file; empty uses bundled tables")
	web := flag.String("web", "demo/web/dist", "built browser files")
	listen := flag.String("listen", "127.0.0.1:2410", "HTTP listen address")
	flag.Parse()
	if _, err := os.Stat(filepath.Join(*web, "index.html")); err != nil {
		log.Fatal("build the browser first: cd demo/web && npm install && npm run build")
	}
	parser, err := blockmesh.Open(blockmesh.OpenOptions{TablesPath: *tables})
	if err != nil {
		log.Fatal(err)
	}
	entries := catalog.Java()
	stateFixer, err := fixer.New()
	if err != nil {
		log.Fatal(err)
	}
	atlasData, err := os.ReadFile(filepath.Join(*assets, "blocksAtlases.json"))
	if err != nil {
		log.Fatal(err)
	}
	var atlas jmodel.Atlas
	if err := json.Unmarshal(atlasData, &atlas); err != nil {
		log.Fatal(err)
	}
	server := &http.Server{
		Addr: *listen,
		Handler: httpapi.New(httpapi.Options{
			Engine:    singleblock.New(parser),
			Subchunks: mesher.NewMesher(parser.Tables),
			Frames:    httpapi.NewFramePipeline(parser.Tables, atlas),
			Fixer:     stateFixer,
			Catalog:   entries,
			AssetsDir: *assets,
			WebDir:    *web,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("Java %s · %d block groups · http://%s", parser.Metadata.Version, len(entries.Groups), *listen)
	log.Fatal(server.ListenAndServe())
}
