package viewhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"neomega-viewer/column"
)

func TestRetireWaitsAndSwitchRejectsOldRequests(t *testing.T) {
	started, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	old := NewView(1, func(ctx context.Context, key column.Key) ([]byte, error) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release
		return []byte("old"), nil
	})
	var builds atomic.Int32
	next := NewView(2, func(ctx context.Context, key column.Key) ([]byte, error) {
		builds.Add(1)
		return []byte("next"), nil
	})
	defer next.Retire()
	var current atomic.Pointer[View]
	current.Store(old)
	var calls atomic.Int32
	handler := Handler(func() *View {
		calls.Add(1)
		return current.Load()
	}, Options{Workers: 1})
	request := func(revision string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/chunk?world="+revision+"&x=0&z=0&minY=0&maxY=0", nil))
		return response
	}
	oldResponse := make(chan *httptest.ResponseRecorder, 1)
	go func() { oldResponse <- request("1") }()
	<-started
	current.Store(next)
	retired := make(chan struct{})
	go func() {
		old.Retire()
		close(retired)
	}()
	<-cancelled
	select {
	case <-retired:
		t.Fatal("Retire returned while build still held the source")
	default:
	}
	if response := request("1"); response.Code != http.StatusConflict {
		t.Fatal(response.Code)
	}
	close(release)
	response := <-oldResponse
	if response.Code != http.StatusConflict || response.Body.String() == "old" {
		t.Fatal(response.Code, response.Body.String())
	}
	<-retired
	if old.cache != nil {
		t.Fatal("retired cache retained")
	}
	for range 2 {
		response = request("2")
		if response.Code != http.StatusOK || response.Body.String() != "next" {
			t.Fatal(response.Code, response.Body.String())
		}
	}
	if builds.Load() != 1 {
		t.Fatal("new View cache not used")
	}
	if calls.Load() != 4 {
		t.Fatalf("current called %d times for 4 requests", calls.Load())
	}
	// 即使调用方刚拿到旧指针才退役，也不能再进入构建。
	staleHandler := Handler(func() *View { return old }, Options{})
	response = httptest.NewRecorder()
	staleHandler.ServeHTTP(response, httptest.NewRequest("GET", "/api/chunk?world=1&x=0&z=0&minY=0&maxY=0", nil))
	if response.Code != http.StatusConflict {
		t.Fatal(response.Code)
	}
}

func TestRetireCancelsQueueAndPreservesWorkerLimit(t *testing.T) {
	// 等所有 goroutine 确实阻塞后再观察队列，避免用延时猜测 worker 是否已经开始。
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		oldBuilds, nextBuilds := 0, 0
		old := NewView(1, func(ctx context.Context, key column.Key) ([]byte, error) {
			oldBuilds++
			<-ctx.Done()
			<-release
			return nil, ctx.Err()
		})
		next := NewView(2, func(ctx context.Context, key column.Key) ([]byte, error) {
			nextBuilds++
			return []byte("next"), nil
		})
		defer next.Retire()
		var current atomic.Pointer[View]
		current.Store(old)
		handler := Handler(current.Load, Options{Workers: 1})
		request := func(query string) *httptest.ResponseRecorder {
			response := httptest.NewRecorder()
			go handler.ServeHTTP(response, httptest.NewRequest(
				"GET",
				"/api/chunk?"+query+"&z=0&minY=0&maxY=0",
				nil,
			))
			return response
		}
		activeResponse := request("world=1&x=0")
		synctest.Wait()
		queuedResponse := request("world=1&x=1")
		synctest.Wait()
		if oldBuilds != 1 {
			t.Errorf("one worker started %d builds", oldBuilds)
		}

		current.Store(next)
		retired := false
		go func() {
			old.Retire()
			retired = true
		}()
		synctest.Wait()
		if queuedResponse.Code != http.StatusConflict || retired {
			t.Errorf("queued response = %d, retired before build ended = %v", queuedResponse.Code, retired)
		}
		nextResponse := request("world=2&x=0")
		synctest.Wait()
		if nextBuilds != 0 {
			t.Error("replacement bypassed handler worker limit")
		}

		close(release)
		synctest.Wait()
		if !retired || oldBuilds != 1 || nextBuilds != 1 {
			t.Fatalf("retired = %v, old builds = %d, next builds = %d", retired, oldBuilds, nextBuilds)
		}
		if activeResponse.Code != http.StatusConflict || nextResponse.Code != http.StatusOK {
			t.Fatal(activeResponse.Code, nextResponse.Code)
		}
	})
}

type blockedResponse struct {
	*httptest.ResponseRecorder
	release chan struct{}
	writing bool
}

func (w *blockedResponse) Write(data []byte) (int, error) {
	w.writing = true
	<-w.release
	return w.ResponseRecorder.Write(data)
}

func TestSlowResponseDoesNotHoldSourceOrWorker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		old := NewView(1, func(ctx context.Context, key column.Key) ([]byte, error) {
			return []byte("old"), nil
		})
		next := NewView(2, func(ctx context.Context, key column.Key) ([]byte, error) {
			return []byte("next"), nil
		})
		defer next.Retire()
		var current atomic.Pointer[View]
		current.Store(old)
		handler := Handler(current.Load, Options{Workers: 1})
		response := &blockedResponse{
			ResponseRecorder: httptest.NewRecorder(),
			release:          make(chan struct{}),
		}
		go handler.ServeHTTP(response, httptest.NewRequest(
			"GET",
			"/api/chunk?world=1&x=0&z=0&minY=0&maxY=0",
			nil,
		))
		synctest.Wait()
		if !response.writing {
			t.Error("response never reached the slow client")
		}

		current.Store(next)
		retired := false
		go func() {
			old.Retire()
			retired = true
		}()
		synctest.Wait()
		if !retired {
			t.Error("Retire waited for client instead of source access")
		}
		nextResponse := httptest.NewRecorder()
		go handler.ServeHTTP(nextResponse, httptest.NewRequest(
			"GET",
			"/api/chunk?world=2&x=0&z=0&minY=0&maxY=0",
			nil,
		))
		synctest.Wait()
		if nextResponse.Body.String() != "next" {
			t.Error("slow response retained the worker")
		}
		close(response.release)
		synctest.Wait()
		if response.Body.String() != "old" {
			t.Error("retirement changed bytes already being sent")
		}
	})
}
