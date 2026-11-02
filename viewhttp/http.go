package viewhttp

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"

	"neomega-viewer/column"
)

type Options struct {
	// 所有 View 共用队列和 worker 限额；资源与页面可以由任意文件系统提供。
	// Assets 挂在 /resources/，Web 挂在 /；Workers 为零时使用四个 worker。
	Assets, Web fs.FS
	Workers     int
}

var (
	errRetired   = errors.New("存档已切换，请重新打开页面")
	errQueueFull = errors.New("render queue busy")
)

type columnHandler struct {
	// 调度属于 Handler，任何一代 View 都使用这些名额。
	current func() *View
	slots   chan struct{}
	queue   chan struct{}
}

func Handler(current func() *View, options Options) http.Handler {
	// 每个列请求只读取一次 current，缓存命中、排队与构建都沿用该 View。
	// current 始终返回非 nil 的 View；空场景也用一个 View 表达。
	workers := options.Workers
	if workers <= 0 {
		workers = 4
	}
	columns := &columnHandler{
		current: current,
		slots:   make(chan struct{}, workers),
		queue:   make(chan struct{}, workers+8),
	}
	mux := http.NewServeMux()
	mux.Handle("GET /api/chunk", columns)
	mux.Handle("GET /resources/", http.StripPrefix("/resources/", http.FileServerFS(options.Assets)))
	mux.Handle("GET /", http.FileServerFS(options.Web))

	return http.NewCrossOriginProtection().Handler(mux)
}

func (h *columnHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key, err := parseKey(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	view := h.current()
	revision, err := strconv.ParseUint(r.URL.Query().Get("world"), 10, 64)
	if err != nil || revision != view.revision {
		stale(w)
		return
	}
	data, err := h.load(r.Context(), view, key)
	switch {
	case errors.Is(err, errRetired) || view.ctx.Err() != nil:
		stale(w)
		return
	case r.Context().Err() != nil:
		return
	case errors.Is(err, errQueueFull):
		w.Header().Set("Retry-After", "1")
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.neomega.world-column")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-store")
	w.Write(data)
}

func (h *columnHandler) load(parent context.Context, view *View, key column.Key) ([]byte, error) {
	// 退役只等待仍可能访问来源的任务，响应发送不占用来源生命周期或 worker。
	if !view.enter() {
		return nil, errRetired
	}
	defer view.active.Done()
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(view.ctx, cancel)
	defer stop()
	defer cancel()
	view.mu.Lock()
	data, ok := view.cache.Get(key)
	view.mu.Unlock()
	if ok {
		return data, nil
	}
	select {
	case h.queue <- struct{}{}:
		defer func() { <-h.queue }()
	default:
		return nil, errQueueFull
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	// 名额与取消可能同时就绪，取得名额后仍需确认这一代可以继续构建。
	if view.ctx.Err() != nil {
		return nil, errRetired
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	view.mu.Lock()
	data, ok = view.cache.Get(key)
	view.mu.Unlock()
	if ok {
		return data, nil
	}
	data, err := view.build(ctx, key)
	if err != nil {
		return nil, err
	}
	if view.ctx.Err() != nil {
		return nil, errRetired
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	view.mu.Lock()
	view.cache.Put(key, data, len(data))
	view.mu.Unlock()
	return data, nil
}

func parseKey(r *http.Request) (column.Key, error) {
	var values [4]int32
	for i, name := range []string{"x", "z", "minY", "maxY"} {
		n, err := strconv.ParseInt(r.URL.Query().Get(name), 10, 32)
		if err != nil {
			return column.Key{}, fmt.Errorf("invalid %s", name)
		}
		values[i] = int32(n)
	}
	if values[0] < -1875000 || values[0] > 1874999 || values[1] < -1875000 || values[1] > 1874999 {
		return column.Key{}, fmt.Errorf("chunk outside world bounds")
	}
	if values[2] < -128 || values[3] > 127 || values[3] < values[2] || values[3]-values[2] >= 15 {
		return column.Key{}, fmt.Errorf("invalid section range")
	}
	return column.Key{
		X:    values[0],
		Z:    values[1],
		MinY: values[2],
		MaxY: values[3],
	}, nil
}

func stale(w http.ResponseWriter) {
	http.Error(w, errRetired.Error(), http.StatusConflict)
}
