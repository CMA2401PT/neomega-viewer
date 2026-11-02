// Generate the same demo layer and binary mesh the browser can load.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"celler/layer_storage"
	"neomega-viewer-core/viewer/mesher"
	"neomega-viewer-core/viewer/mesher/meshing"
	blockmesh "neomega-viewer-core/viewer/resources"
	"neomega-viewer/demo/scenes"
	"neomega-viewer/demo/subchunk"
)

func main() {
	tables := flag.String("tables", "", "geometry table file; empty uses bundled tables")
	scene := flag.String("scene", "mixed", "demo recipe")
	input := flag.String("input", "", "read a 4096-ID little-endian block layer instead of a demo")
	blocksOut := flag.String("blocks", "", "write the logical block layer as .nvblocks")
	meshOut := flag.String("mesh", "/tmp/subchunk.nvmesh", "output binary mesh")
	cull := flag.Bool("cull", true, "remove covered boundary faces")
	greedy := flag.Bool("greedy", true, "merge compatible rectangles")
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
	parser, err := blockmesh.Open(blockmesh.OpenOptions{TablesPath: *tables})
	if err != nil {
		log.Fatal(err)
	}
	var plain layer_storage.Plain[uint32]
	plain.CopyFrom(layer)
	result, err := mesher.NewMesher(parser.Tables).MeshSubchunk(plain.Data, mesher.Options{
		Meshing: meshing.Options{
			DisableCulling: !*cull,
			DisableGreedy:  !*greedy,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	file, err := os.Create(*meshOut)
	if err != nil {
		log.Fatal(err)
	}
	if err := result.Mesh.Marshal(file); err != nil {
		file.Close()
		log.Fatal(err)
	}
	if err := file.Close(); err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result.Stats); err != nil {
		log.Fatal(err)
	}
}
