// Package httpapi transports static block geometry. The rendering library itself
// has no dependency on HTTP, the browser, or the demo catalog.
package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	java "neomega-viewer-core/viewer/javastates"
	"neomega-viewer-core/fixer/java-postprocess"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer-core/viewer/mesher"
	"neomega-viewer/demo/catalog"
	"neomega-viewer/demo/singleblock"
)

type Options struct {
	Engine    *singleblock.Engine
	Subchunks *mesher.Mesher[uint32]
	Frames    *dispatcher.Pipeline
	Fixer     *fixer.Fixer
	Catalog   *catalog.Catalog
	AssetsDir string
	WebDir    string
}

type request struct {
	ID uint32 `json:"id"`
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
		addSubchunks(mux, options.Subchunks, options.Fixer, options.Engine.Metadata.Version)
	}
	if options.Frames != nil {
		addFrames(mux, options.Frames, options.Fixer)
	}
	mux.HandleFunc("GET /api/blocks", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, struct {
			Version  string          `json:"version"`
			AtlasURL string          `json:"atlasUrl"`
			Groups   []catalog.Group `json:"groups"`
		}{Version: options.Engine.Metadata.Version, AtlasURL: "/assets/atlas.png", Groups: options.Catalog.Groups})
	})
	mux.HandleFunc("GET /api/states", func(w http.ResponseWriter, r *http.Request) {
		states, ok := options.Catalog.ByName[r.URL.Query().Get("name")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		responses := make([]StateResponse, 0, len(states))
		for _, block := range states {
			responses = append(responses, stateResponse(block))
		}
		writeJSON(w, http.StatusOK, responses)
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseUint(r.URL.Query().Get("id"), 10, 32)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		binding, ok := java.HashToBlock(uint32(id))
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, stateResponse(binding))
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
		block, err := options.Engine.Build(input.ID)
		if err != nil {
			writeError(w, 422, err)
			return
		}
		writeJSON(w, 200, blockResponse(block))
	}
	mux.HandleFunc("GET /api/block", build)
	mux.HandleFunc("POST /api/block", build)
	mux.HandleFunc("GET /assets/atlas.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(options.AssetsDir, "atlas.png"))
	})
	mux.HandleFunc("GET /assets/THIRD_PARTY_NOTICES.txt", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(options.AssetsDir, "THIRD_PARTY_NOTICES.txt"))
	})
	mux.Handle("GET /", cachedFiles(http.Dir(options.WebDir)))
	return mux
}

// 构建产物里带哈希的 assets 可长期缓存，但入口 HTML 必须每次校验，
// 否则重建后浏览器会拿着旧 HTML 去请求已不存在的旧 JS，整页起不来。
func cachedFiles(root http.FileSystem) http.Handler {
	files := http.FileServer(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}
