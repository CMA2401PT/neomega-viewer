package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io/fs"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"celler"
	"celler/java"
	"celler/layer_storage"
	"celler/storage"
	"neomega-blocks/ir"
	packfs "neomega-packfs"
	"neomega-viewer-core/viewer/blockdata"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer-core/viewer/merger"
	preset "neomega-viewer-core/viewer/resources"
	renderresources "neomega-viewer-core/viewer/resources"
	"neomega-viewer/column"
	"neomega-viewer/viewassets"
)

func TestWorldDirectoryAndPack(t *testing.T) {
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := newPipeline(resources.Tables)
	if err != nil {
		t.Fatal(err)
	}
	for _, backend := range []string{"directory", "namespaced", "pack"} {
		t.Run(backend, func(t *testing.T) {
			root := t.TempDir()
			var files storage.Writer
			var container *packfs.PackFS
			options := SourceOptions{Directory: root}
			if backend == "pack" {
				options = SourceOptions{Pack: filepath.Join(root, "world.pack")}
				container, err = packfs.Open(options.Pack)
				if err != nil {
					t.Fatal(err)
				}
				files = storage.Pack{PackFS: container}
			} else {
				files, err = storage.OpenDirectory(root)
				if err != nil {
					t.Fatal(err)
				}
			}
			dimension := java.Overworld
			if backend == "namespaced" {
				dimension = "dimensions/minecraft/overworld"
			}
			world, err := java.Open(files, java.Options{})
			if err != nil {
				t.Fatal(err)
			}
			pos := celler.ChunkPos{X: -1, Z: 2}
			chunk := java.NewChunk(pos, 4189)
			// Two solid sections share a hidden horizontal face; only ten outer quads remain.
			for _, y := range []int32{4, 5} {
				if err = chunk.SetSection(&java.Section{Y: y, Blocks: layer_storage.MakePaletteStorage(ir.IRBlock("minecraft:stone []"))}); err != nil {
					t.Fatal(err)
				}
			}
			if err = world.WriteChunk(dimension, chunk); err != nil {
				t.Fatal(err)
			}
			if err = world.Commit(); err != nil {
				t.Fatal(err)
			}
			if err = world.Close(); err != nil {
				t.Fatal(err)
			}
			if container != nil {
				if err = container.Close(); err != nil {
					t.Fatal(err)
				}
			}
			source, err := OpenSource(options)
			if err != nil {
				t.Fatal(err)
			}
			defer source.Close()
			if source.Info.Dimension != string(dimension) || source.Info.Position[0] != -8 || source.Info.Position[2] != 40 {
				t.Fatalf("initial view: %+v", source.Info)
			}
			packet, err := column.BuildColumn(
				context.Background(),
				engine,
				source.Reader(),
				column.Key{
					X:    -1,
					Z:    2,
					MinY: 2,
					MaxY: 6,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if string(packet[:4]) != "NVWC" || binary.LittleEndian.Uint32(packet[8:]) != 2 {
				t.Fatalf("packet header %x", packet[:16])
			}
			offset, quads := 20, 0
			frames := map[int32][]byte{}
			for range 2 {
				size := int(binary.LittleEndian.Uint32(packet[offset+4:]))
				y := int32(binary.LittleEndian.Uint32(packet[offset:]))
				frames[y] = packet[offset+12 : offset+12+size]
				frame, err := merger.Unmarshal(bytes.NewReader(packet[offset+12 : offset+12+size]))
				if err != nil {
					t.Fatal(err)
				}
				quads += len(frame.Mesh.Quads)
				offset += 12 + size
			}
			if offset != len(packet) || quads != 10 {
				t.Fatalf("got %d quads, packet consumed %d/%d", quads, offset, len(packet))
			}
			for y, frame := range frames {
				single, err := column.BuildColumn(
					context.Background(),
					engine,
					source.Reader(),
					column.Key{
						X:    -1,
						Z:    2,
						MinY: y,
						MaxY: y,
					},
				)
				if err != nil {
					t.Fatal(err)
				}
				if binary.LittleEndian.Uint32(single[8:]) != 1 || !bytes.Equal(single[32:], frame) {
					t.Fatal("section geometry depends on request window")
				}
			}
			handler := Handler(ServerOptions{Worlds: NewWorlds(source, engine)})
			request := httptest.NewRequest("GET", "/api/chunk?world=1&x=-1&z=2&minY=2&maxY=6", nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != 200 || !bytes.Equal(response.Body.Bytes(), packet) {
				t.Fatalf("HTTP column: %d", response.Code)
			}
			for _, query := range []string{"x=0&z=0&minY=2&maxY=99", "x=0&z=0&minY=999&maxY=999", "x=bad&z=0&minY=2&maxY=6"} {
				response = httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/chunk?"+query, nil))
				if response.Code != 400 {
					t.Fatal(response.Code)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err = source.Reader().Section(ctx, dispatcher.SectionPos{
				X: pos.X,
				Z: pos.Z,
			}); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestEmptySourceAndUnknownState(t *testing.T) {
	source, err := OpenSource(SourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if source.Info.Loaded {
		t.Fatal("empty source is loaded")
	}
	if section, err := source.Reader().Section(context.Background(), dispatcher.SectionPos{}); err != nil || section != nil {
		t.Fatal(section, err)
	}
	source.Close()
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := newPipeline(resources.Tables)
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Build(
		dispatcher.Section{Blocks: blockdata.Uniform(4096, ir.IRBlock("example:unknown []"))},
		dispatcher.Neighbours{},
		dispatcher.Options{},
	)
	if err != nil || len(result.Diagnostics) != 4096 || len(result.Frame.Mesh.Quads) == 0 {
		t.Fatal("unknown blocks disappeared")
	}
}

func TestEmbeddedSiteAndResources(t *testing.T) {
	source, err := OpenSource(SourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	assets := renderresources.Resources()
	web := viewassets.Web()
	handler := Handler(ServerOptions{
		Worlds: NewWorlds(source, nil),
		Assets: assets,
		Web:    web,
	})
	for _, name := range []string{"/", "/resources/atlas.png", "/resources/THIRD_PARTY_NOTICES.txt", "/LICENSE.txt", "/THREE-LICENSE.txt"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", name, nil))
		if response.Code != 200 || response.Body.Len() == 0 {
			t.Fatalf("embedded %s: %d", name, response.Code)
		}
	}
	// Vite scripts use /assets; texture routes must not shadow those files.
	entries, err := fs.ReadDir(web, "assets")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/assets/"+entry.Name(), nil))
		if response.Code != 200 {
			t.Fatalf("embedded script %s: %d", entry.Name(), response.Code)
		}
	}
}
