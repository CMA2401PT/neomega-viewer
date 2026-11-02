package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	"celler/layer_storage"
	"neomega-blocks/ir"
	preset "neomega-blocks/presets/java/v1/geometry"
	"neomega-viewer/column"
	"neomega-viewer/dispatcher"
	"neomega-viewer/source"
	"neomega-viewer/viewassets"
	"neomega-viewer/viewhttp"
)

func main() {
	// 内存来源与存档浏览共用列构建和网页，只由程序提供固定的初始视点。
	listen := flag.String("listen", "127.0.0.1:2412", "HTTP listen address")
	flag.Parse()
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		log.Fatal(err)
	}
	pipeline := dispatcher.NewJavaPipeline(resources.Tables, nil, nil)
	sections := source.MemorySections{}
	for x := int32(0); x < 2; x++ {
		blocks := layer_storage.MakePaletteStorage(ir.IRBlock("minecraft:air []"))
		for z := 2; z < 14; z++ {
			for bx := 2; bx < 14; bx++ {
				blocks.Set(layer_storage.At(bx, 0, z), ir.IRBlock("minecraft:stone []"))
			}
		}
		blocks.Set(layer_storage.At(8, 1, 8), ir.IRBlock("minecraft:stone []"))
		blocks.Set(layer_storage.At(8, 2, 8), ir.IRBlock("minecraft:enchanting_table []"))
		sections[dispatcher.SectionPos{X: x}] = &dispatcher.Section{Blocks: blocks}
	}
	view := viewhttp.NewView(1, func(ctx context.Context, key column.Key) ([]byte, error) {
		return column.BuildColumn(
			ctx,
			pipeline,
			sections,
			key,
		)
	})
	defer view.Retire()
	mux := http.NewServeMux()
	mux.Handle("GET /api/world", viewhttp.DescriptionHandler(viewhttp.Description{
		Revision:        1,
		Loaded:          true,
		Name:            "内存 section",
		Position:        [3]float64{16, 14, 28},
		ResourceVersion: resources.Metadata.Version,
	}))
	mux.Handle("/", viewhttp.Handler(
		func() *viewhttp.View { return view },
		viewhttp.Options{
			Assets: viewassets.Resources(),
			Web:    viewassets.Web(),
		},
	))
	log.Printf("http://%s", *listen)
	log.Fatal(http.ListenAndServe(*listen, mux))
}
