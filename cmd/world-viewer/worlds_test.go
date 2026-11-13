package main

import (
	"context"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer/column"
	"neomega-viewer/viewhttp"
)

func TestSwitchPublishesBeforeWaitingAndClosesAfterBuild(t *testing.T) {
	started, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var closed atomic.Bool
	previous := &openedContent{
		Info: Info{
			Description: viewhttp.Description{
				Revision: 1,
				Name:     "previous",
			},
		},
		View: viewhttp.NewView(1, func(ctx context.Context, key column.Key) ([]byte, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			<-release
			if closed.Load() {
				t.Error("source closed while build was still reading")
			}
			return nil, ctx.Err()
		}),
		close: func() { closed.Store(true) },
	}
	worlds := &Worlds{}
	worlds.current.Store(previous)
	handler := Handler(ServerOptions{Worlds: worlds})
	response := httptest.NewRecorder()
	requestDone := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/chunk?world=1&x=0&z=0&minY=0&maxY=0", nil))
		close(requestDone)
	}()
	<-started
	switched := make(chan error, 1)
	go func() {
		_, err := worlds.Open(context.Background(), SourceOptions{})
		switched <- err
	}()
	<-cancelled
	current := worlds.view()
	if current == previous || current.Info.Revision != 2 || current.Info.Loaded || closed.Load() {
		t.Fatal("new content not published as one object before retirement")
	}
	close(release)
	if err := <-switched; err != nil {
		t.Fatal(err)
	}
	<-requestDone
	if !closed.Load() || response.Code != 409 {
		t.Fatal("old source not retired then closed")
	}
	worlds.Close()
}

func TestMinimapOnlyDisablesMeshBuilds(t *testing.T) {
	// 隔离模式不申请精细 reader，切换内容后仍不创建列构建代次。
	source, err := OpenSource(context.Background(), SourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	source.reader = func() dispatcher.SectionSource {
		t.Fatal("仅小地图模式不应读取精细网格来源")
		return nil
	}
	worlds := NewWorlds(source, nil)
	defer worlds.Close()
	handler := Handler(ServerOptions{Worlds: worlds})
	for range 2 {
		if !worlds.view().Info.MinimapOnly || worlds.view().View != nil || worlds.view().Minimap == nil {
			t.Fatal("仅小地图模式应保留地图并停用列构建")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/chunk?world=1&x=0&z=0&minY=0&maxY=0", nil))
		if response.Code != 404 {
			t.Fatalf("精细网格接口应停用，收到 %d", response.Code)
		}
		if _, err := worlds.Open(context.Background(), SourceOptions{}); err != nil {
			t.Fatal(err)
		}
	}
}
