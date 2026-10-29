package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"neomega-viewer/blockmesh"
	"neomega-viewer/catalog"
	"neomega-viewer/httpapi"
	"neomega-viewer/singleblock"
	"neomega-viewer/subchunk"
)

func main() {
	assets := flag.String("assets", "../neomega-assets/dist/final", "matching resource bundle")
	web := flag.String("web", "web/dist", "built browser files")
	listen := flag.String("listen", "127.0.0.1:2410", "HTTP listen address")
	flag.Parse()
	if _, err := os.Stat(filepath.Join(*web, "index.html")); err != nil {
		log.Fatal("build the browser first: cd web && npm install && npm run build")
	}
	parser, err := blockmesh.Open(blockmesh.OpenOptions{AssetsDir: *assets})
	if err != nil {
		log.Fatal(err)
	}
	entries, err := catalog.NEMC()
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{
		Addr: *listen,
		Handler: httpapi.New(httpapi.Options{
			Engine:    singleblock.New(parser),
			Subchunks: subchunk.New(parser),
			Catalog:   entries,
			AssetsDir: *assets,
			WebDir:    *web,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("Java %s · %d block groups · http://%s", parser.Assets.BuildInfo.Version, len(entries.Groups), *listen)
	log.Fatal(server.ListenAndServe())
}
