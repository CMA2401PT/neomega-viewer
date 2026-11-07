package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"testing/synctest"

	"celler"
	"neomega-viewer-core/viewer/blockdata"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer/column"
	"neomega-viewer/minimap"
	"neomega-viewer/source"
	"neomega-viewer/viewhttp"
)

func TestMinimapRetirementBeforeSourceClose(t *testing.T) {
	for _, action := range []string{"index", "tile"} {
		for _, switchWorld := range []bool{false, true} {
			t.Run(action+"/"+map[bool]string{true: "switch", false: "close"}[switchWorld], func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					started, release := make(chan struct{}), make(chan struct{})
					closed := false
					waitForRelease := func(ctx context.Context) {
						close(started)
						<-ctx.Done()
						<-release
						if closed {
							t.Error("小地图仍在读取就关闭来源")
						}
					}
					mapView := minimap.New(minimap.Options{
						Reader: func() dispatcher.SectionSource { return waitingSections{wait: waitForRelease} },
						Chunks: func(ctx context.Context) ([]celler.ChunkPos, error) { waitForRelease(ctx); return nil, ctx.Err() },
					})
					previous := &openedContent{
						Info:    Info{Description: viewhttp.Description{Revision: 1}},
						View:    viewhttp.NewView(1, func(context.Context, column.Key) ([]byte, error) { return nil, nil }),
						Minimap: mapView,
						close:   func() { closed = true },
					}
					worlds := &Worlds{}
					worlds.current.Store(previous)
					handler := Handler(ServerOptions{Worlds: worlds})
					response := httptest.NewRecorder()
					go handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/minimap/"+action+"?world=1&kind=detail&x=0&z=0", nil))
					<-started
					finished := false
					go func() {
						if switchWorld {
							if _, err := worlds.Open(context.Background(), SourceOptions{}); err != nil {
								t.Error(err)
							}
						} else {
							worlds.Close()
						}
						finished = true
					}()
					synctest.Wait()
					if closed || finished {
						t.Fatal("没有等待小地图读取完成")
					}
					if switchWorld && worlds.view() == previous {
						t.Fatal("退役等待前没有发布新内容")
					}
					rejected := httptest.NewRecorder()
					handler.ServeHTTP(rejected, httptest.NewRequest("GET", "/api/minimap/index?world=1", nil))
					if rejected.Code != 409 {
						t.Fatal("退役后未拒绝新任务", rejected.Code)
					}
					close(release)
					synctest.Wait()
					if !closed || !finished || response.Code != 409 {
						t.Fatal(closed, finished, response.Code)
					}
					if switchWorld {
						worlds.Close()
					}
				})
			})
		}
	}
}

func TestMinimapHTTPAndIndexCache(t *testing.T) {
	scans := 0
	sections := source.MemorySections{
		{X: -1}: &dispatcher.Section{Blocks: blockdata.Uniform(4096, "minecraft:stone []")},
	}
	src := &Source{minimap: &minimap.Options{
		Terrain: minimap.Terrain{MinSection: 0, MaxSection: 0},
		Reader:  func() dispatcher.SectionSource { return sections },
		Chunks:  func(context.Context) ([]celler.ChunkPos, error) { scans++; return []celler.ChunkPos{{X: -1}}, nil },
	}, close: func() {}}
	worlds := NewWorlds(src, nil)
	defer worlds.Close()
	if !worlds.view().Info.Minimap {
		t.Fatal("页面缺少小地图能力标记")
	}
	handler := Handler(ServerOptions{Worlds: worlds})
	firstTile := httptest.NewRecorder()
	handler.ServeHTTP(firstTile, httptest.NewRequest("GET", "/api/minimap/tile?world=1&kind=detail&x=-1&z=0", nil))
	if firstTile.Code != 200 || scans != 0 {
		t.Fatal("近景不应等待整个世界的索引", firstTile.Code, scans)
	}
	for _, test := range []struct {
		query  string
		status int
	}{
		{"index?world=1", 200},
		{"index?world=1", 200},
		{"tile?world=1&kind=detail&x=-1&z=0", 200},
		{"tile?world=1&kind=overview&x=-1&z=0", 200},
		{"surface?world=1&x=-1&z=0", 200},
		{"surface?world=1&x=200&z=0", 200},
		{"index?world=0", 409},
		{"index", 400},
		{"tile?world=1&kind=bad&x=0&z=0", 400},
		{"surface?world=1&x=NaN&z=0", 400},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/minimap/"+test.query, nil))
		if response.Code != test.status {
			t.Fatal(test, response.Code, response.Body.String())
		}
		if test.query == "surface?world=1&x=-1&z=0" {
			var point struct {
				Y *int32 `json:"y"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &point); err != nil || point.Y == nil || *point.Y != 15 {
				t.Fatal(point, err)
			}
		}
		if test.query == "surface?world=1&x=200&z=0" && response.Body.String() != `{"x":200,"z":0,"y":null}` {
			t.Fatal(response.Body.String())
		}
	}
	if scans != 1 {
		t.Fatal("索引被重复扫描", scans)
	}
}

type waitingSections struct {
	// 模拟已经收到取消、但尚未归还底层句柄的图块读取。
	wait func(context.Context)
}

func (s waitingSections) Section(ctx context.Context, pos dispatcher.SectionPos) (*dispatcher.Section, error) {
	s.wait(ctx)
	return nil, ctx.Err()
}

func TestMinimapIndexWaitersLeaveBothTileSlotsFree(t *testing.T) {
	// 多个页面等待同一次索引时，两张图块仍可同时构建。
	synctest.Test(t, func(t *testing.T) {
		indexRelease, tileRelease := make(chan struct{}), make(chan struct{})
		scans := 0
		startedTiles := make(chan struct{}, 2)
		m := minimap.New(minimap.Options{
			Terrain: minimap.Terrain{MinSection: 0, MaxSection: 0},
			Chunks:  func(context.Context) ([]celler.ChunkPos, error) { scans++; <-indexRelease; return nil, nil },
			Reader: func() dispatcher.SectionSource {
				return tileSlotSource{read: func() { startedTiles <- struct{}{}; <-tileRelease }}
			},
		})
		worlds := &Worlds{}
		worlds.current.Store(&openedContent{Info: Info{Description: viewhttp.Description{Revision: 1}}, Minimap: m})
		handler := Handler(ServerOptions{Worlds: worlds})
		responses := make([]*httptest.ResponseRecorder, 4)
		for i, path := range []string{"index?world=1", "index?world=1", "tile?world=1&kind=detail&x=0&z=0", "tile?world=1&kind=detail&x=1&z=0"} {
			responses[i] = httptest.NewRecorder()
			go handler.ServeHTTP(responses[i], httptest.NewRequest("GET", "/api/minimap/"+path, nil))
		}
		synctest.Wait()
		if scans != 1 || len(startedTiles) != 2 {
			t.Fatal("索引等待占用了图块名额", scans, len(startedTiles))
		}
		close(tileRelease)
		close(indexRelease)
		synctest.Wait()
		for _, response := range responses {
			if response.Code != 200 {
				t.Fatal(response.Code, response.Body.String())
			}
		}
		m.Retire()
	})
}

type tileSlotSource struct {
	// 只阻塞本次图块的第一根柱子，让测试直接观察并发构建的数量。
	read func()
}

func (s tileSlotSource) Section(ctx context.Context, pos dispatcher.SectionPos) (*dispatcher.Section, error) {
	if pos.X%8 == 0 && pos.Z == 0 {
		s.read()
	}
	return &dispatcher.Section{Blocks: blockdata.Uniform(4096, "minecraft:stone []")}, nil
}
