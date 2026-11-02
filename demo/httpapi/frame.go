package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"celler/layer_storage"
	"neomega-blocks/ir"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer-core/fixer/java-postprocess"
	"neomega-viewer-core/viewer/dispatcher"
	blockgeometry "neomega-viewer-core/viewer/geometry"
	jmodel "neomega-viewer-core/viewer/geometry/java_block_model"
	"neomega-viewer-core/viewer/merger"
	"neomega-viewer/demo/scenes"
	"neomega-viewer/demo/subchunk"
)

func NewFramePipeline(tables *blockgeometry.Tables[uint32], atlas jmodel.Atlas) *dispatcher.Pipeline {
	// demo 使用共享的 Java 处理器组合，缺失模型继续由 mesher 返回错误。
	return dispatcher.NewJavaPipeline(tables, &atlas, nil)
}

func addFrames(mux *http.ServeMux, pipeline *dispatcher.Pipeline, stateFixer *fixer.Fixer) {
	send := func(
		w http.ResponseWriter,
		r *http.Request,
		blocks *layer_storage.Palette[uint32],
		entities map[[3]uint8]nbt.Compound,
	) {
		options, err := sectionOptions(r)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		var plain layer_storage.Plain[uint32]
		plain.CopyFrom(blocks)
		states, err := dispatcher.BlocksFromHashes(plain.Data)
		if err != nil {
			writeError(w, 422, err)
			return
		}
		if r.URL.Query().Get("fix") == "1" {
			// 独立预览使用六面 section 入口，缺失邻居由规则按空气或缺数据处理。
			states = stateFixer.FixSection(states, [6]*layer_storage.Palette[ir.IRBlock]{})
		}
		result, err := pipeline.Build(
			dispatcher.Section{Blocks: states, Entities: entities},
			dispatcher.Neighbours{},
			dispatcher.Options{Mesh: options},
		)
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
