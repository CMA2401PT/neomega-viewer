package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"celler/layer_storage"

	"neomega-viewer/demo"
	"neomega-viewer/meshwire"
	"neomega-viewer/subchunk"
)

func sectionOptions(r *http.Request) (subchunk.Options, error) {
	var options subchunk.Options
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

func addSubchunks(mux *http.ServeMux, engine *subchunk.Engine) {
	mux.HandleFunc("GET /api/subchunks", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, struct {
			Version  string       `json:"version"`
			AtlasURL string       `json:"atlasUrl"`
			Scenes   []demo.Scene `json:"scenes"`
		}{engine.Blocks.Assets.BuildInfo.Version, "/assets/atlas.png", demo.List()})
	})
	send := func(w http.ResponseWriter, r *http.Request, layer *layer_storage.Palette[uint32]) {
		options, err := sectionOptions(r)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		var plain layer_storage.Plain[uint32]
		plain.CopyFrom(layer)
		result, err := engine.Build(&plain, options)
		if err != nil {
			writeError(w, 422, err)
			return
		}
		encoded, err := meshwire.Encode(result.Mesh.Buffers())
		if err != nil {
			writeError(w, 500, err)
			return
		}
		stats, _ := json.Marshal(result.Stats)
		w.Header().Set("Content-Type", meshwire.ContentType)
		w.Header().Set("X-Neomega-Mesh-Stats", string(stats))
		w.Header().Set("Content-Length", strconv.Itoa(len(encoded)))
		w.Write(encoded)
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
