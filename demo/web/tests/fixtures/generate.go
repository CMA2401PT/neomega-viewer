//go:build ignore

// 从仓库根执行 go run demo/web/tests/fixtures/generate.go，生成与图集匹配的真实快照。
package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"

	"celler/layer_storage"
	jmodel "neomega-blocks/geometry/java_block_model"
	blockmesh "neomega-blocks/presets/java/v1/geometry"
	"neomega-viewer/demo/httpapi"
	"neomega-viewer/demo/scenes"
	"neomega-viewer/dispatcher"
)

func main() {
	assets := flag.String("assets", "../neomega-assets/dist/final", "matching resource bundle")
	output := flag.String("out", "demo/web/tests/fixtures/block_entities.nvframe", "frame output")
	flag.Parse()
	data, err := os.ReadFile(filepath.Join(*assets, "blocksAtlases.json"))
	if err != nil {
		log.Fatal(err)
	}
	var atlas jmodel.Atlas
	if err := json.Unmarshal(data, &atlas); err != nil {
		log.Fatal(err)
	}
	parser, err := blockmesh.Open(blockmesh.OpenOptions{})
	if err != nil {
		log.Fatal(err)
	}
	scene, err := demo.BuildWithNBT("block_entities")
	if err != nil {
		log.Fatal(err)
	}
	var plain layer_storage.Plain[uint32]
	plain.CopyFrom(scene.Blocks)
	pipeline := httpapi.NewFramePipeline(parser.Tables, atlas)
	result, err := pipeline.Build(plain.Data, scene.NBT, dispatcher.Options{})
	if err != nil {
		log.Fatal(err)
	}
	file, err := os.Create(*output)
	if err != nil {
		log.Fatal(err)
	}
	if err := result.Frame.Marshal(file); err != nil {
		log.Fatal(err)
	}
	if err := file.Close(); err != nil {
		log.Fatal(err)
	}
	log.Printf("%s: static=%d quads, actors=%d, effects=%d", *output, len(result.Frame.Mesh.Quads), len(result.Frame.Actors), len(result.Frame.Effects))
}
