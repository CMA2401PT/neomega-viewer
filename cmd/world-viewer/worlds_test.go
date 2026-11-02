package main

import (
	"context"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"neomega-viewer/column"
	"neomega-viewer/viewhttp"
)

func TestSwitchPublishesBeforeWaitingAndClosesAfterBuild(t *testing.T) {
	started, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var closed atomic.Bool
	previous := &openedContent{
		Info: Info{
			Revision: 1,
			Name:     "previous",
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
