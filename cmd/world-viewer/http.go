package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"neomega-converter-core/structure"
	"net/http"
	"strings"

	"neomega-viewer/viewhttp"
)

type ServerOptions struct {
	StructureMemoryMiB int
	StructureWorkers   int
	Worlds             *Worlds
	Assets, Web        fs.FS
	ResourceVersion    string
	Workers            int
	SkipBedrockFix     bool
}

func Handler(options ServerOptions) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/world", func(w http.ResponseWriter, r *http.Request) {
		info := options.Worlds.view().Info
		info.ResourceVersion = options.ResourceVersion
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(info)
	})
	mux.HandleFunc("GET /api/files", func(w http.ResponseWriter, r *http.Request) {
		listing, err := browseDirectory(r.URL.Query().Get("path"))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(listing)
	})
	mux.HandleFunc("POST /api/world", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "expected application/json", 415)
			return
		}
		var input struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&input); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		source, err := sourceAtPath(input.Path)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		source.SkipBedrockFix = options.SkipBedrockFix
		source.StructureMemoryMiB = options.StructureMemoryMiB
		source.StructureWorkers = options.StructureWorkers
		// 流式响应沿用同一次打开请求，进度只属于当前调用者；断开连接会取消转换。
		if strings.Contains(r.Header.Get("Accept"), "application/x-ndjson") {
			ctx, cancel := context.WithCancel(r.Context())
			defer cancel()
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.Header().Set("Cache-Control", "no-store")
			encoder := json.NewEncoder(w)
			emit := func(event openEvent) {
				if err := encoder.Encode(event); err != nil {
					cancel()
					return
				}
				if err := http.NewResponseController(w).Flush(); err != nil {
					cancel()
				}
			}
			emit(openEvent{Type: "progress", Progress: &structure.Progress{Stage: "opening"}})
			source.Progress = func(progress structure.Progress) { emit(openEvent{Type: "progress", Progress: &progress}) }
			info, err := options.Worlds.Open(ctx, source)
			if err != nil {
				emit(openEvent{Type: "error", Error: err.Error()})
				return
			}
			info.ResourceVersion = options.ResourceVersion
			emit(openEvent{Type: "complete", Info: &info})
			return
		}
		info, err := options.Worlds.Open(r.Context(), source)
		if err != nil {
			http.Error(w, err.Error(), 422)
			return
		}
		info.ResourceVersion = options.ResourceVersion
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(info)
	})

	mux.Handle("/", viewhttp.Handler(
		func() *viewhttp.View { return options.Worlds.view().View },
		viewhttp.Options{
			Assets:  options.Assets,
			Web:     options.Web,
			Workers: options.Workers,
		},
	))
	return http.NewCrossOriginProtection().Handler(mux)
}

// openEvent 是 POST 打开请求的逐行响应，成功前失败不会替换当前存档。
type openEvent struct {
	Type     string              `json:"type"`
	Progress *structure.Progress `json:"progress,omitempty"`
	Info     *Info               `json:"info,omitempty"`
	Error    string              `json:"error,omitempty"`
}
