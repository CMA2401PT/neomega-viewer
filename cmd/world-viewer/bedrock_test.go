package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"

	"celler"
	"celler/bedrock"
	"celler/layer_storage"
	"neomega-converter-core/adapt"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer-core/viewer/merger"
	preset "neomega-viewer-core/viewer/resources"
	"neomega-viewer/column"
	"neomega-viewer/viewhttp"
)

func TestBedrockFixtureBuildColumn(t *testing.T) {
	// LevelDB 只打开临时副本，样例目录始终作为普通文件复制来源。
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../../../../resource/基岩版中地图")); err != nil {
		t.Fatal(err)
	}
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := newPipeline(resources.Tables)
	if err != nil {
		t.Fatal(err)
	}
	for _, skip := range []bool{true, false} {
		t.Run(fmt.Sprintf("skip_fix=%v", skip), func(t *testing.T) {
			src, err := OpenSource(SourceOptions{
				Directory:      dir,
				SkipBedrockFix: skip,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer src.Close()
			key := column.Key{
				X:    int32(src.Info.Position[0]) >> 4,
				Z:    int32(src.Info.Position[2]) >> 4,
				MinY: -4,
				MaxY: 19,
			}
			packet, err := column.BuildColumn(
				context.Background(),
				pipeline,
				src.Reader(),
				key,
			)
			if err != nil {
				t.Fatal(err)
			}
			frames := binary.LittleEndian.Uint32(packet[8:])
			if frames == 0 {
				t.Fatalf("fixture column %+v has no mesh", key)
			}
			frameSize := int(binary.LittleEndian.Uint32(packet[24:]))
			frame, err := merger.Unmarshal(bytes.NewReader(packet[32 : 32+frameSize]))
			if err != nil || len(frame.Mesh.Quads) == 0 {
				t.Fatal("fixture produced no static geometry", err)
			}
			t.Logf("column (%d,%d): %d mesh frames, %d bytes", key.X, key.Z, frames, len(packet))
		})
	}
}

func writeBedrockWorld(t *testing.T, withLevel bool) string {
	// 两根跨边界栅栏和两个其他维度，让位置选择和修复开关都能观察到实际结果。
	t.Helper()
	dir := t.TempDir()
	world, err := bedrock.Open(dir, bedrock.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer world.Close()
	for _, entry := range []struct {
		dimension int32
		x         int32
		localX    int
	}{
		{
			dimension: 0,
			x:         -1,
			localX:    15,
		},
		{
			dimension: 0,
			x:         0,
			localX:    0,
		},
		{
			dimension: 1,
			x:         4,
			localX:    0,
		},
		{
			dimension: 2,
			x:         8,
			localX:    0,
		},
	} {
		chunk := bedrock.NewChunk(celler.ChunkPos{
			X: entry.x,
			Z: 2,
		}, entry.dimension)
		layer := bedrock.NewLayer(bedrock.Block{
			State:   "minecraft:air []",
			Version: 18168865,
		})
		layer.Blocks.Set(layer_storage.At(entry.localX, 4, 8), bedrock.Block{
			State:   "minecraft:oak_fence []",
			Version: 18168865,
		})
		if err := chunk.SetSection(&bedrock.Section{
			Y:       0,
			Version: 9,
			Layers:  []*bedrock.Layer{layer},
		}); err != nil {
			t.Fatal(err)
		}
		if err := world.WriteChunk(chunk, nil); err != nil {
			t.Fatal(err)
		}
	}
	if withLevel {
		if err := world.WriteLevel(bedrock.Level{
			Version: 10,
			NBT: nbt.Document{Value: nbt.Compound{
				"SpawnX": int32(-1),
				"SpawnY": int32(10),
				"SpawnZ": int32(40),
			}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestBedrockOpenDimensionsAndSpawn(t *testing.T) {
	dir := writeBedrockWorld(t, true)
	for _, test := range []struct {
		dimension string
		wantDim   string
		position  [3]float64
	}{
		{
			dimension: "",
			wantDim:   "0",
			position:  [3]float64{-1, 22, 40},
		},
		{
			dimension: "0",
			wantDim:   "0",
			position:  [3]float64{-1, 22, 40},
		},
		{
			dimension: "1",
			wantDim:   "1",
			position:  [3]float64{72, 24, 40},
		},
		{
			dimension: "2",
			wantDim:   "2",
			position:  [3]float64{136, 24, 40},
		},
	} {
		src, err := OpenSource(SourceOptions{
			Directory:      dir,
			Dimension:      test.dimension,
			SkipBedrockFix: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if src.Info.Kind != "bedrock" || src.Info.Dimension != test.wantDim || src.Info.Position != test.position {
			t.Fatalf("dimension %q: %+v", test.dimension, src.Info)
		}
		src.Close()
	}
	position := [3]float64{1, 2, 3}
	src, err := OpenSource(SourceOptions{
		Directory: dir,
		Position:  &position,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if src.Info.Position != position {
		t.Fatal(src.Info.Position)
	}
	for _, invalid := range []string{"DIM-1", "-1", "3"} {
		if _, err := OpenSource(SourceOptions{
			Directory: dir,
			Dimension: invalid,
		}); err == nil {
			t.Fatalf("accepted Bedrock dimension %q", invalid)
		}
	}
}

func TestBedrockPickerAndFixSetting(t *testing.T) {
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := newPipeline(resources.Tables)
	if err != nil {
		t.Fatal(err)
	}
	for _, withLevel := range []bool{false, true} {
		dir := writeBedrockWorld(t, withLevel)
		paths := []string{dir}
		if withLevel {
			paths = append(paths, filepath.Join(dir, "level.dat"))
		}
		for _, path := range paths {
			options, err := sourceAtPath(path)
			if err != nil || options.Directory != dir {
				t.Fatal(options, err)
			}
		}
		for _, skip := range []bool{true, false} {
			src, err := OpenSource(SourceOptions{
				Directory:      dir,
				SkipBedrockFix: skip,
			})
			if err != nil {
				t.Fatal(err)
			}
			section, err := src.Reader().Section(context.Background(), dispatcher.SectionPos{
				X: -1,
				Z: 2,
			})
			if err != nil {
				t.Fatal(err)
			}
			state, _ := section.Blocks.Get(15<<8 | 8<<4 | 4).States().Get("east")
			if state != fmt.Sprint(!skip) {
				t.Fatalf("skip fix %v: east = %v", skip, state)
			}
			want, err := column.BuildColumn(
				context.Background(),
				pipeline,
				src.Reader(),
				column.Key{
					X: -1,
					Z: 2,
				},
			)
			src.Close()
			if err != nil {
				t.Fatal(err)
			}
			empty, err := OpenSource(SourceOptions{})
			if err != nil {
				t.Fatal(err)
			}
			worlds := NewWorlds(empty, pipeline)
			handler := Handler(ServerOptions{
				Worlds:         worlds,
				SkipBedrockFix: skip,
			})
			body, _ := json.Marshal(map[string]string{"path": paths[len(paths)-1]})
			request := httptest.NewRequest("POST", "/api/world", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != 200 {
				t.Fatal(response.Code, response.Body.String())
			}
			var info Info
			if err := json.Unmarshal(response.Body.Bytes(), &info); err != nil {
				t.Fatal(err)
			}
			if info.Kind != "bedrock" || !info.CanOpen || !info.Loaded {
				t.Fatal(info)
			}
			// 重新打开同一目录也遵循先打开后退役的顺序，两个只读句柄会短暂共存。
			request = httptest.NewRequest("POST", "/api/world", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response = httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != 200 {
				t.Fatal("reopening Bedrock world", response.Code, response.Body.String())
			}
			response = httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/chunk?world=3&x=-1&z=2&minY=0&maxY=0", nil))
			worlds.Close()
			if response.Code != 200 || !bytes.Equal(response.Body.Bytes(), want) {
				t.Fatalf("browser open did not preserve fix setting %v: status %d", skip, response.Code)
			}
		}
	}
}

func TestBedrockSwitchWaitsForRawRead(t *testing.T) {
	// adapt 会先返回取消，但模拟的底层 I/O 不响应取消，必须等它归还句柄。
	synctest.Test(t, func(t *testing.T) {
		started, release := make(chan struct{}), make(chan struct{})
		closed := false
		reads := &bedrockReads{read: func(ctx context.Context, pos celler.ChunkPos) (*bedrock.Chunk, error) {
			close(started)
			<-release
			if closed {
				t.Error("LevelDB closed before raw read finished")
			}
			return nil, fs.ErrNotExist
		}}
		converted := adapt.NewConverted(reads.Read, adapt.Options{})
		previous := &openedContent{
			Info: Info{Description: viewhttp.Description{Revision: 1}},
			View: viewhttp.NewView(1, func(ctx context.Context, key column.Key) ([]byte, error) {
				_, err := converted.Chunk(ctx, celler.ChunkPos{})
				return nil, err
			}),
			close: func() {
				reads.Close()
				closed = true
			},
		}
		worlds := &Worlds{}
		worlds.current.Store(previous)
		response := httptest.NewRecorder()
		go Handler(ServerOptions{Worlds: worlds}).ServeHTTP(response, httptest.NewRequest("GET", "/api/chunk?world=1&x=0&z=0&minY=0&maxY=0", nil))
		<-started
		switched := false
		go func() {
			if _, err := worlds.Open(context.Background(), SourceOptions{}); err != nil {
				t.Error(err)
			}
			switched = true
		}()
		synctest.Wait()
		if closed || switched || worlds.view() == previous || response.Code != 409 {
			t.Fatal("switch must publish, cancel the old request, then wait for raw I/O")
		}
		close(release)
		synctest.Wait()
		if !closed || !switched {
			t.Fatal("switch did not close after read finished")
		}
		if _, err := reads.Read(context.Background(), celler.ChunkPos{}); !errors.Is(err, fs.ErrClosed) {
			t.Fatal("late adapt callback entered a closed database", err)
		}
		worlds.Close()
	})
}
