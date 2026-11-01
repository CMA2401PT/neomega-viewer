// HTTP endpoints for the world viewer application.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
)

type ServerOptions struct {
	Worlds          *Worlds
	Engine          *Engine
	Assets, Web     fs.FS
	ResourceVersion string
	Workers         int
}

func Handler(options ServerOptions) http.Handler {
	workers := options.Workers
	if workers <= 0 {
		workers = 2
	}
	slots := make(chan struct{}, workers)
	queue := make(chan struct{}, workers+8)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/world", func(w http.ResponseWriter, r *http.Request) {
		info := options.Worlds.view().info()
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
	mux.HandleFunc("GET /api/chunk", func(w http.ResponseWriter, r *http.Request) {
		key, err := parseKey(r)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		view := options.Worlds.view()
		revision, err := strconv.ParseUint(r.URL.Query().Get("world"), 10, 64)
		if err != nil || revision != view.revision {
			http.Error(w, "存档已切换，请重新打开页面", 409)
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		stop := context.AfterFunc(view.ctx, cancel)
		defer stop()
		defer cancel()
		cache := view.cache
		view.mu.Lock()
		data, ok := cache.get(key)
		view.mu.Unlock()
		if !ok {
			select {
			case queue <- struct{}{}:
				defer func() { <-queue }()
			default:
				w.Header().Set("Retry-After", "1")
				http.Error(w, "render queue busy", 503)
				return
			}
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				return
			}
			view.mu.Lock()
			data, ok = cache.get(key)
			view.mu.Unlock()
			if !ok {
				data, err = options.Engine.BuildColumn(ctx, view.source, key)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					http.Error(w, err.Error(), 422)
					return
				}
				if ctx.Err() != nil {
					return
				}
				view.mu.Lock()
				cache.put(key, data, len(data))
				view.mu.Unlock()
			}
		}
		w.Header().Set("Content-Type", "application/vnd.neomega.world-column")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Cache-Control", "no-store")
		w.Write(data)
	})
	mux.Handle("GET /resources/", http.StripPrefix("/resources/", http.FileServerFS(options.Assets)))
	mux.Handle("GET /", http.FileServerFS(options.Web))

	return http.NewCrossOriginProtection().Handler(mux)
}
func parseKey(r *http.Request) (chunkKey, error) {
	var values [4]int32
	for i, name := range []string{"x", "z", "minY", "maxY"} {
		n, err := strconv.ParseInt(r.URL.Query().Get(name), 10, 32)
		if err != nil {
			return chunkKey{}, fmt.Errorf("invalid %s", name)
		}
		values[i] = int32(n)
	}
	if values[0] < -1875000 || values[0] > 1874999 || values[1] < -1875000 || values[1] > 1874999 {
		return chunkKey{}, fmt.Errorf("chunk outside world bounds")
	}
	if values[2] < -128 || values[3] > 127 || values[3] < values[2] || values[3]-values[2] >= 15 {
		return chunkKey{}, fmt.Errorf("invalid section range")
	}
	return chunkKey{X: values[0], Z: values[1], MinY: values[2], MaxY: values[3]}, nil
}
