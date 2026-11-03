package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"neomega-viewer-core/fixer/java-postprocess"
	"neomega-viewer-core/viewer/cube"
	"neomega-viewer-core/viewer/javapreset"
	"neomega-viewer-core/viewer/mesher"
	blockmesh "neomega-viewer-core/viewer/resources"
	"neomega-viewer/demo/catalog"
	"neomega-viewer/demo/httpapi"
	"neomega-viewer/demo/singleblock"
)

func main() {
	assets := flag.String("assets", "../../neomega-viewer-core/viewer/resources/textures", "matching resource bundle")
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
	pipeline, err := javapreset.NewPipeline(cube.New(16), parser.Tables, parser.Items, nil)
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{
		Addr: *listen,
		Handler: httpapi.New(httpapi.Options{
			Engine:    singleblock.New(parser),
			Subchunks: mesher.NewMesher(cube.New(16), parser.Tables),
			Frames:    pipeline,
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
