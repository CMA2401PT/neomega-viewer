package minimap

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"celler"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer/source"
)

func TestIndexSurvivesWaiterCancellation(t *testing.T) {
	// 等待者可以更换，扫描仍属于同一代，且只发布一次结果。
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		scans := 0
		m := New(Options{Chunks: func(ctx context.Context) ([]celler.ChunkPos, error) {
			scans++
			<-release
			if ctx.Err() != nil {
				t.Error("请求取消传播到了地图扫描", ctx.Err())
			}
			return []celler.ChunkPos{{X: -1, Z: 0}}, nil
		}})
		defer m.Retire()
		ctx, cancel := context.WithCancel(context.Background())
		var firstErr error
		go func() { _, firstErr = m.Index(ctx) }()
		synctest.Wait()
		cancel()
		synctest.Wait()
		if !errors.Is(firstErr, context.Canceled) {
			t.Fatal(firstErr)
		}
		var index *Index
		go func() {
			var err error
			index, err = m.Index(context.Background())
			if err != nil {
				t.Error(err)
			}
		}()
		synctest.Wait()
		if scans != 1 || index != nil {
			t.Fatal("重复启动扫描或提前返回", scans, index)
		}
		close(release)
		synctest.Wait()
		cached, err := m.Index(context.Background())
		if err != nil || index == nil || cached != index || scans != 1 {
			t.Fatal(index, cached, err, scans)
		}
		if len(index.Detail) != 1 || index.Detail[0] != (Position{X: -1, Z: 0}) {
			t.Fatal(index)
		}
	})
}

func TestRetireKeepsCompletedData(t *testing.T) {
	// 退役只结束来源访问，已经生成的图片和索引随 Map 一起释放。
	m := New(Options{
		Terrain: Terrain{MinSection: 0, MaxSection: 0},
		Reader:  func() dispatcher.SectionSource { return source.MemorySections{} },
		Chunks:  func(context.Context) ([]celler.ChunkPos, error) { return []celler.ChunkPos{{X: 1}}, nil },
	})
	index, err := m.Index(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	before, err := m.Tile(context.Background(), "detail", Position{})
	if err != nil {
		t.Fatal(err)
	}
	m.Retire()
	after, err := m.Tile(context.Background(), "detail", Position{})
	if err != nil || !bytes.Equal(before, after) || m.index != index || !m.chunks[celler.ChunkPos{X: 1}] {
		t.Fatal("退役不应清空已完成数据", err)
	}
}

func TestTerrainDimensions(t *testing.T) {
	for _, test := range []struct {
		dimension  Dimension
		minY, maxY int32
		nether     bool
	}{
		{Overworld, -4, 19, false}, {Nether, 0, 7, true}, {End, 0, 15, false},
	} {
		terrain, err := NewTerrain(test.dimension)
		if err != nil || terrain.MinSection != test.minY || terrain.MaxSection != test.maxY || terrain.Nether != test.nether || len(terrain.Colors) == 0 {
			t.Fatal(terrain, err)
		}
	}
}
