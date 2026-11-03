//go:build ignore

// 从仓库根执行 go run demo/web/tests/fixtures/generate.go，生成与图集匹配的真实快照。
package main

import (
	"flag"
	"log"
	"os"

	"celler/layer_storage"
	"neomega-viewer-core/viewer/cube"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer-core/viewer/javapreset"
	blockmesh "neomega-viewer-core/viewer/resources"
	"neomega-viewer/demo/scenes"
	viewsource "neomega-viewer/source"
)

func main() {
	output := flag.String("out", "../../neomega-viewer-core/viewer/web/tests/fixtures/block_entities.nvframe", "frame output")
	flag.Parse()
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
	pipeline, err := javapreset.NewPipeline(cube.New(16), parser.Tables, parser.Items, nil)
	if err != nil {
		log.Fatal(err)
	}
	states, err := viewsource.PaletteFromHashes(plain.Data)
	if err != nil {
		log.Fatal(err)
	}
	result, err := pipeline.Build(dispatcher.Section{Blocks: viewsource.BlocksFromPalette(states), BlockEntities: scene.NBT}, dispatcher.Neighbours{}, dispatcher.Options{})
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
