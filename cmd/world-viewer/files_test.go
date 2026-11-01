package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"celler"
	"celler/java"
	"celler/layer_storage"
	"neomega-blocks/ir"
	preset "neomega-blocks/presets/java/v1/geometry"
	packfs "neomega-packfs"
)

func writePickerWorld(t *testing.T, directory string, block ir.IRBlock) {
	t.Helper()
	world, err := java.OpenDirectory(directory, java.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer world.Close()
	chunk := java.NewChunk(celler.ChunkPos{}, 4189)
	if err = chunk.SetSection(&java.Section{Y: 4, Blocks: layer_storage.MakePaletteStorage(block)}); err != nil {
		t.Fatal(err)
	}
	if err = world.WriteChunk(java.Overworld, chunk); err != nil {
		t.Fatal(err)
	}
	if err = world.Commit(); err != nil {
		t.Fatal(err)
	}
}
func TestWorldSwitchIsolatesCachesAndRejectsStaleRequests(t *testing.T) {
	stone, air := t.TempDir(), t.TempDir()
	writePickerWorld(t, stone, "minecraft:stone []")
	writePickerWorld(t, air, "minecraft:air []")
	source, err := OpenSource(SourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	worlds := NewWorlds(source)
	defer worlds.Close()
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(resources.Tables, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(ServerOptions{Worlds: worlds, Engine: engine})
	open := func(path string, code int) Info {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"path": path})
		request := httptest.NewRequest("POST", "/api/world", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != code {
			t.Fatalf("open %s: %d %s", path, response.Code, response.Body.String())
		}
		var info Info
		if code == 200 {
			if err := json.Unmarshal(response.Body.Bytes(), &info); err != nil {
				t.Fatal(err)
			}
		}
		return info
	}
	if info := open(stone, 200); info.Revision != 2 || info.Path != stone {
		t.Fatal(info)
	}
	old := worlds.view()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/chunk?world=2&x=0&z=0&minY=4&maxY=4", nil))
	if response.Code != 200 || response.Body.Len() <= 20 {
		t.Fatal("first world's mesh missing")
	}
	if info := open(air, 200); info.Revision != 3 || info.Path != air {
		t.Fatal(info)
	}
	if old.ctx.Err() != context.Canceled {
		t.Fatal("old jobs not cancelled")
	}
	select {
	case <-old.source.done:
	default:
		t.Fatal("old file owner not closed")
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/chunk?world=2&x=0&z=0&minY=4&maxY=4", nil))
	if response.Code != 409 {
		t.Fatal("old browser accepted")
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/chunk?world=3&x=0&z=0&minY=4&maxY=4", nil))
	if response.Code != 200 || response.Body.Len() != 20 {
		t.Fatal("old mesh leaked into new world")
	}
	open(filepath.Join(air, "missing"), 400)
	broken := filepath.Join(t.TempDir(), "broken.pack")
	if err = os.WriteFile(broken, []byte("not a PackFS container"), 0644); err != nil {
		t.Fatal(err)
	}
	open(broken, 422)
	if worlds.view().revision != 3 || worlds.view().source.Info.Path != air {
		t.Fatal("failed open replaced active world")
	}
	// A rejected candidate cannot change the current world either.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := worlds.Open(ctx, SourceOptions{Directory: stone}); err != context.Canceled {
		t.Fatal(err)
	}
	if worlds.view().revision != 3 {
		t.Fatal("cancelled switch committed")
	}
}

func TestFileBrowserPathsAndPackSelection(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "地图 # 中文 & spaces")
	if err := os.Mkdir(directory, 0755); err != nil {
		t.Fatal(err)
	}
	writePickerWorld(t, directory, "minecraft:stone []")
	level := filepath.Join(directory, "level.dat")
	if err := os.WriteFile(level, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{directory, "\"" + directory + "\"", level} {
		options, err := sourceAtPath(name)
		if err != nil || options.Directory != directory {
			t.Fatal(options, err)
		}
	}
	source, err := OpenSource(SourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	worlds := NewWorlds(source)
	defer worlds.Close()
	handler := Handler(ServerOptions{Worlds: worlds})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/files?"+url.Values{"path": {level}}.Encode(), nil))
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var listing directoryListing
	if err = json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	if listing.Path != directory || listing.Selected != level || listing.Entries[0].Name != "region" {
		t.Fatalf("listing: %+v", listing)
	}
	packPath := filepath.Join(directory, "test.pack")
	container, err := packfs.Open(packPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = container.Close(); err != nil {
		t.Fatal(err)
	}
	options, err := sourceAtPath(packPath)
	if err != nil || options.Pack != packPath {
		t.Fatal(options, err)
	}
	empty := filepath.Join(directory, "empty.pack")
	if err = os.WriteFile(empty, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = sourceAtPath(empty); err == nil {
		t.Fatal("empty file accepted as pack")
	}
	stat, err := os.Stat(empty)
	if err != nil || stat.Size() != 0 {
		t.Fatal("selection mutated file")
	}
	if _, err = sourceAtPath(filepath.Join(directory, "missing")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err = browseDirectory(filepath.Join(directory, "missing")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err = sourceAtPath(t.TempDir()); err == nil {
		t.Fatal("unrelated directory accepted")
	}
	if _, err = fs.Stat(os.DirFS(directory), "region"); err != nil {
		t.Fatal(err)
	}
}
