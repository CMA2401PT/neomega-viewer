package javaworld

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"slices"
	"sync"
	"testing"

	"celler"
	"celler/java"
	"celler/java/region"
)

func TestChunksSerializedWithReaders(t *testing.T) {
	world, err := java.OpenDirectory(t.TempDir(), java.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer world.Close()
	want := []celler.ChunkPos{{X: -33, Z: 4}, {X: 0, Z: 0}, {X: 32, Z: -1}}
	// 索引只依赖 region 头；刻意放入不能解码为 NBT 的记录。
	for _, pos := range want {
		if err := world.WriteRaw(java.Overworld, java.Terrain, pos, region.Record{Compression: 3, Data: []byte("header only")}); err != nil {
			t.Fatal(err)
		}
	}
	if err := world.WriteRaw(java.Nether, java.Terrain, celler.ChunkPos{X: 99}, region.Record{Compression: 3, Data: []byte("other dimension")}); err != nil {
		t.Fatal(err)
	}
	if err := world.Commit(); err != nil {
		t.Fatal(err)
	}
	src := New(world, java.Overworld)
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			got, err := src.Chunks(context.Background())
			if err != nil {
				t.Error(err)
				return
			}
			slices.SortFunc(got, func(a, b celler.ChunkPos) int { return int(a.X - b.X) })
			if !reflect.DeepEqual(got, want) {
				t.Errorf("索引 %v，期望 %v", got, want)
			}
			if _, err := src.read(context.Background(), want[0]); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := src.Chunks(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	src.Close()
	if _, err := src.Chunks(context.Background()); !errors.Is(err, fs.ErrClosed) {
		t.Fatal(err)
	}
}
