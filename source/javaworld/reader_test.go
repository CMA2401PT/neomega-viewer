package javaworld

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"
	"testing/synctest"

	"celler"
	"celler/java"
	"celler/layer_storage"
	"neomega-blocks/ir"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer-core/viewer/blockactor"
	"neomega-viewer-core/viewer/cube"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer-core/viewer/effector"
	preset "neomega-viewer-core/viewer/resources"
	"neomega-viewer/column"
)

func TestReaderDiagnosticsBelongToRequestedColumn(t *testing.T) {
	world, err := java.OpenDirectory(t.TempDir(), java.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer world.Close()
	for _, x := range []int32{0, 1} {
		chunk := java.NewChunk(celler.ChunkPos{X: x}, 4189)
		chunk.NBT.Value["block_entities"] = []nbt.Compound{
			{
				"id": "example:one",
				"x":  "bad",
				"y":  int32(200),
				"z":  int32(0),
			},
			{
				"id": "example:two",
				"x":  x * 16,
				"y":  int32(-200),
				"z":  "bad",
			},
		}
		if err = chunk.SetSection(&java.Section{
			Y:      0,
			Blocks: layer_storage.MakePaletteStorage(ir.IRBlock("minecraft:stone []")),
		}); err != nil {
			t.Fatal(err)
		}
		if err = world.WriteChunk(java.Overworld, chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err = world.Commit(); err != nil {
		t.Fatal(err)
	}
	s := New(world, java.Overworld)
	defer s.Close()
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pipeline := dispatcher.NewPipeline(cube.New(16), resources.Tables, effector.New(nil), blockactor.New(), nil)
	for _, y := range []int32{0, 4} {
		reader := s.Reader()
		packet, err := column.BuildColumn(
			context.Background(),
			pipeline,
			reader,
			column.Key{
				MinY: y,
				MaxY: y,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		size := int(binary.LittleEndian.Uint32(packet[16:]))
		var issues []column.Issue
		if err = json.Unmarshal(packet[len(packet)-size:], &issues); err != nil {
			t.Fatal(err)
		}
		if len(issues) != 2 {
			t.Fatalf("center issues merged or neighbor issues leaked: %+v", issues)
		}
		for i, issue := range issues {
			if issue.Scope != "column" || issue.Position != [3]int32{} || issue.Count != 1 || issue.Reason != "invalid_block_entity" {
				t.Fatal(issue)
			}
			if i > 0 && issue.Detail == issues[0].Detail {
				t.Fatal("distinct entity errors merged")
			}
		}
	}
	// 并发 reader 共享原始记录，各自持有解码结果。
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			reader := s.Reader()
			section, err := reader.Section(context.Background(), dispatcher.SectionPos{})
			if err != nil || section == nil {
				t.Errorf("read section: %v, %v", section, err)
			}
		})
	}
	workers.Wait()
}

func TestReadCancellationAndClose(t *testing.T) {
	world, err := java.OpenDirectory(t.TempDir(), java.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer world.Close()
	s := New(world, java.Overworld)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.read(ctx, celler.ChunkPos{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = s.read(context.Background(), celler.ChunkPos{}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	s.Close()
	if _, err = s.read(context.Background(), celler.ChunkPos{}); !errors.Is(err, fs.ErrClosed) {
		t.Fatal(err)
	}
}

type blockedRegionFS struct {
	fs.FS
	release chan struct{}
}

func (f blockedRegionFS) Open(name string) (fs.File, error) {
	if name == "region/r.0.0.mca" {
		<-f.release
	}
	return f.FS.Open(name)
}

func TestCloseWaitsForIOAfterReaderCancellation(t *testing.T) {
	// 模拟不能由 context 中断的文件访问，验证 reader 返回不等于句柄已经空闲。
	synctest.Test(t, func(t *testing.T) {
		files := blockedRegionFS{
			FS:      fstest.MapFS{},
			release: make(chan struct{}),
		}
		world, err := java.Open(files, java.Options{ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		defer world.Close()
		s := New(world, java.Overworld)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var readErr error
		go func() {
			_, readErr = s.Reader().Section(ctx, dispatcher.SectionPos{})
		}()
		synctest.Wait()
		cancel()
		synctest.Wait()
		if !errors.Is(readErr, context.Canceled) {
			t.Errorf("reader cancellation: %v", readErr)
		}
		closed := false
		go func() {
			s.Close()
			closed = true
		}()
		synctest.Wait()
		if closed {
			t.Error("source returned World while file access was still blocked")
		}
		close(files.release)
		synctest.Wait()
		if !closed {
			t.Fatal("source did not finish closing after I/O returned")
		}
	})
}
