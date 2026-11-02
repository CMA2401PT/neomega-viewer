package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"

	"neomega-viewer/viewhttp"
)

type ServerOptions struct {
	Worlds          *Worlds
	Assets, Web     fs.FS
	ResourceVersion string
	Workers         int
	SkipBedrockFix  bool
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
