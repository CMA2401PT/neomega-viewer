// Package httpapi transports static block geometry. The rendering library itself
// has no dependency on HTTP, the browser, or the demo catalog.
package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"

	"neomega-viewer/catalog"
	"neomega-viewer/model"
	"neomega-viewer/singleblock"
	"neomega-viewer/subchunk"
)

type Options struct {
	Engine    *singleblock.Engine
	Subchunks *subchunk.Engine
	Catalog   *catalog.Catalog
	AssetsDir string
	WebDir    string
}

type request struct {
	ID uint32 `json:"id"`
	singleblock.Options
}

type response struct {
	singleblock.Packet
	PropertyValues map[string][]string `json:"propertyValues"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func New(options Options) http.Handler {
	mux := http.NewServeMux()
	if options.Subchunks != nil {
		addSubchunks(mux, options.Subchunks)
	}
	mux.HandleFunc("GET /api/blocks", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, struct {
			Version  string          `json:"version"`
			AtlasURL string          `json:"atlasUrl"`
			Groups   []catalog.Group `json:"groups"`
		}{Version: options.Engine.Assets.BuildInfo.Version, AtlasURL: "/assets/atlas.png", Groups: options.Catalog.Groups})
	})
	mux.HandleFunc("GET /api/states", func(w http.ResponseWriter, r *http.Request) {
		states, ok := options.Catalog.ByName[r.URL.Query().Get("name")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, states)
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		binding, ok := options.Catalog.ByID[uint32(id)]
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, binding)
	})
	build := func(w http.ResponseWriter, r *http.Request) {
		var input request
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&input); err != nil {
				writeError(w, 400, err)
				return
			}
		} else {
			id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
			if err != nil {
				writeError(w, 400, err)
				return
			}
			input.ID = uint32(id)
		}
		block, err := options.Engine.Build(input.ID, input.Options)
		if err != nil {
			writeError(w, 422, err)
			return
		}
		definition, _ := options.Engine.Assets.BlockState(block.State.Name)
		writeJSON(w, 200, response{Packet: block.Packet(), PropertyValues: model.PropertyValues(definition, block.State.Properties)})
	}
	mux.HandleFunc("GET /api/block", build)
	mux.HandleFunc("POST /api/block", build)
	mux.HandleFunc("GET /assets/atlas.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(options.AssetsDir, "blocksAtlasLatest.png"))
	})
	mux.HandleFunc("GET /assets/THIRD_PARTY_NOTICES.txt", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(options.AssetsDir, "THIRD_PARTY_NOTICES.txt"))
	})
	mux.Handle("GET /", http.FileServer(http.Dir(options.WebDir)))
	return mux
}
