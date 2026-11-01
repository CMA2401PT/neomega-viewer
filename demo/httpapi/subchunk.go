package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"celler/layer_storage"

	"neomega-blocks/ir"
	java "neomega-blocks/presets/java/v1"
	"neomega-converter/fixer"
	"neomega-viewer/demo/scenes"
	"neomega-viewer/demo/subchunk"
	"neomega-viewer/dispatcher"
	"neomega-viewer/mesher"
	"neomega-viewer/mesher/geometry"
	"neomega-viewer/splitlayer"
)

func sectionOptions(r *http.Request) (mesher.Options, error) {
	var options mesher.Options
	for _, flag := range []struct {
		Name   string
		Target *bool
	}{{"cull", &options.Meshing.DisableCulling}, {"greedy", &options.Meshing.DisableGreedy}} {
		if text := r.URL.Query().Get(flag.Name); text != "" {
			value, err := strconv.ParseBool(text)
			if err != nil {
				return options, err
			}
			*flag.Target = !value
		}
	}
	return options, nil
}

func addSubchunks(mux *http.ServeMux, engine *mesher.Mesher[uint32], stateFixer *fixer.Fixer, version string) {
	layerSplitter := splitlayer.New()
	mux.HandleFunc("GET /api/subchunks", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, struct {
			Version  string       `json:"version"`
			AtlasURL string       `json:"atlasUrl"`
			Scenes   []demo.Scene `json:"scenes"`
		}{version, "/assets/atlas.png", demo.List()})
	})
	send := func(w http.ResponseWriter, r *http.Request, layer *layer_storage.Palette[uint32]) {
		options, err := sectionOptions(r)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		var plain layer_storage.Plain[uint32]
		plain.CopyFrom(layer)
		layers := layerSplitter.SplitSubchunk(plain.Data)
		if r.URL.Query().Get("fix") == "1" {
			// 独立预览没有相邻子区块，只修复当前层内的连接。
			states, err := dispatcher.BlocksFromHashes(layers[0])
			if err != nil {
				writeError(w, 422, err)
				return
			}
			fixed := stateFixer.FixSection(states, [6]*layer_storage.Palette[ir.IRBlock]{})
			var repaired layer_storage.Plain[uint32]
			repaired.CopyFrom(layer_storage.ConvertPalette(fixed, java.BlockToHash))
			layers[0] = repaired.Data
		}
		result, err := engine.MeshMultiLayerSubchunks(layers, options)
		if err != nil {
			writeError(w, 422, err)
			return
		}
		stats, _ := json.Marshal(result.Stats)
		w.Header().Set("Content-Type", geometry.ContentType)
		w.Header().Set("X-Neomega-Mesh-Stats", string(stats))
		if err := result.Mesh.Marshal(w); err != nil {
			// 正文可能已经开始，交由 HTTP 中止响应，避免混入 JSON。
			panic(http.ErrAbortHandler)
		}
	}
	mux.HandleFunc("GET /api/subchunks/{scene}/mesh", func(w http.ResponseWriter, r *http.Request) {
		layer, err := demo.Build(r.PathValue("scene"))
		if err != nil {
			writeError(w, 404, err)
			return
		}
		send(w, r, layer)
	})
	mux.HandleFunc("GET /api/subchunks/{scene}/blocks", func(w http.ResponseWriter, r *http.Request) {
		layer, err := demo.Build(r.PathValue("scene"))
		if err != nil {
			writeError(w, 404, err)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("scene")+`.nvblocks"`)
		w.Write(subchunk.EncodeIDs(layer))
	})
	mux.HandleFunc("POST /api/subchunk", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, subchunk.IDsByteSize))
		if err != nil {
			writeError(w, 400, err)
			return
		}
		layer, err := subchunk.ParseIDs(data)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		send(w, r, layer)
	})
}
