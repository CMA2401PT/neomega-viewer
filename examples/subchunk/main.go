// Generate the same demo layer and binary mesh the browser can load.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"neomega-viewer/blockmesh"
	"neomega-viewer/demo"
	"neomega-viewer/meshing"
	"neomega-viewer/meshwire"
	"neomega-viewer/subchunk"
)

func main() {
	assets := flag.String("assets", "../neomega-assets/dist/final", "resource directory")
	scene := flag.String("scene", "mixed", "demo recipe")
	input := flag.String("input", "", "read a 4096-ID little-endian block layer instead of a demo")
	blocksOut := flag.String("blocks", "", "write the logical block layer as .nvblocks")
	meshOut := flag.String("mesh", "/tmp/subchunk.nvmesh", "output binary mesh")
	cull := flag.Bool("cull", true, "remove covered boundary faces")
	greedy := flag.Bool("greedy", true, "merge compatible rectangles")
	vary := flag.Bool("vary", false, "vary weighted model choices by cell position")
	seed := flag.Uint64("seed", 0, "static model selection seed")
	flag.Parse()
	layer, err := demo.Build(*scene)
	if *input != "" {
		var data []byte
		data, err = os.ReadFile(*input)
		if err == nil {
			layer, err = subchunk.ParseIDs(data)
		}
	}
	if err != nil {
		log.Fatal(err)
	}
	if *blocksOut != "" {
		if err := os.WriteFile(*blocksOut, subchunk.EncodeIDs(layer), 0644); err != nil {
			log.Fatal(err)
		}
	}
	parser, err := blockmesh.Open(blockmesh.OpenOptions{AssetsDir: *assets})
	if err != nil {
		log.Fatal(err)
	}
	result, err := subchunk.New(parser).Build(layer, subchunk.Options{
		Seed:       *seed,
		VaryModels: *vary,
		Meshing: meshing.Options{
			DisableCulling: !*cull,
			DisableGreedy:  !*greedy,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	data, err := meshwire.Encode(result.Mesh.Buffers())
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*meshOut, data, 0644); err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result.Stats); err != nil {
		log.Fatal(err)
	}
}
