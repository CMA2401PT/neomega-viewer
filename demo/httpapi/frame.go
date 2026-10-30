package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"celler/layer_storage"
	"neomega-assets/asset"
	"neomega-viewer/blockactor"
	"neomega-viewer/blockactor/decoratedpot"
	"neomega-viewer/demo/scenes"
	"neomega-viewer/demo/subchunk"
	"neomega-viewer/dispatcher"
	"neomega-viewer/effector"
	"neomega-viewer/fixer"
	"neomega-viewer/merger"
	"neomega-viewer/mesher"
	"neomega-viewer/mesher/material"
)

func NewFramePipeline(tables *mesher.Tables[uint32], atlas asset.Atlas, stateFixer *fixer.Fixer) *dispatcher.Pipeline {
	// demo 在此启用具体方块，框架只认识处理器接口；资源加载仍由应用入口负责。
	surfaces := func(texture string) (material.Surface, error) {
		name := strings.TrimPrefix(texture, "minecraft:")
		entry, found := atlas.Textures[name]
		if !found {
			return material.Surface{}, fmt.Errorf("atlas texture %s missing", texture)
		}
		return material.Surface{
			Texture: texture,
			Region: [4]float32{
				float32(entry.U),
				float32(entry.V),
				float32(entry.SU),
				float32(entry.SV),
			},
			Mode:      material.Opaque,
			Opacity:   1,
			Frames:    entry.Frames,
			FrameTime: entry.FrameTime,
		}, nil
	}
	return dispatcher.NewPipeline(
		tables,
		effector.New(map[string]string{
			"minecraft:enchanting_table": "enchanting_table_book",
		}),
		blockactor.New(decoratedpot.New(tables, surfaces)),
		func(blocks []uint32) []uint32 {
			return stateFixer.Fix(blocks, fixer.Neighbours{})
		},
	)
}

func addFrames(mux *http.ServeMux, pipeline *dispatcher.Pipeline) {
	send := func(w http.ResponseWriter, r *http.Request, blocks *layer_storage.Palette[uint32], nbt map[[3]uint8]string) {
		options, err := sectionOptions(r)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		var plain layer_storage.Plain[uint32]
		plain.CopyFrom(blocks)
		result, err := pipeline.Build(plain.Data, nbt, dispatcher.Options{
			Mesh: options,
			Fix:  r.URL.Query().Get("fix") == "1",
		})
		if err != nil {
			writeError(w, 422, err)
			return
		}
		stats, _ := json.Marshal(result.Stats)
		w.Header().Set("Content-Type", merger.ContentType)
		w.Header().Set("X-Neomega-Mesh-Stats", string(stats))
		if err := result.Frame.Marshal(w); err != nil {
			// 正文可能已经开始，交由 HTTP 中止响应，避免混入 JSON。
			panic(http.ErrAbortHandler)
		}
	}
	mux.HandleFunc("GET /api/subchunks/{scene}/frame", func(w http.ResponseWriter, r *http.Request) {
		scene, err := demo.BuildWithNBT(r.PathValue("scene"))
		if err != nil {
			writeError(w, 404, err)
			return
		}
		send(w, r, scene.Blocks, scene.NBT)
	})
	mux.HandleFunc("POST /api/subchunk/frame", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, subchunk.IDsByteSize))
		if err != nil {
			writeError(w, 400, err)
			return
		}
		blocks, err := subchunk.ParseIDs(data)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		send(w, r, blocks, nil)
	})
}
