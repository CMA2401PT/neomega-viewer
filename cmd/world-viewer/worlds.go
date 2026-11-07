package main

import (
	"context"
	"sync"
	"sync/atomic"

	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer/column"
	"neomega-viewer/minimap"
	"neomega-viewer/viewhttp"
)

type openedContent struct {
	// 描述、浏览代次与关闭动作一起发布，读取者不会拼出两代内容。
	Info    Info
	View    *viewhttp.View
	Minimap *minimap.Map
	close   func()
}

func newContent(source *Source, revision uint64, pipeline *dispatcher.Pipeline) *openedContent {
	// 构建闭包固定来源，每次调用单独申请 reader，避免请求之间共享解码状态。
	info := source.Info
	info.Revision = revision
	info.Version = version
	var mapView *minimap.Map
	if source.minimap != nil {
		mapView = minimap.New(*source.minimap)
	}
	info.Minimap = mapView != nil
	return &openedContent{
		Info:    info,
		Minimap: mapView,
		View: viewhttp.NewView(revision, func(ctx context.Context, key column.Key) ([]byte, error) {
			return column.BuildColumn(
				ctx,
				pipeline,
				source.Reader(),
				key,
			)
		}),
		close: source.Close,
	}
}

type Worlds struct {
	// 切换串行进行，GET 与列请求只需读取同一个已发布对象。
	switchMu sync.Mutex
	current  atomic.Pointer[openedContent]
	pipeline *dispatcher.Pipeline
}

func NewWorlds(source *Source, pipeline *dispatcher.Pipeline) *Worlds {
	w := &Worlds{pipeline: pipeline}
	w.current.Store(newContent(source, 1, pipeline))
	return w
}
func (w *Worlds) view() *openedContent {
	return w.current.Load()
}

func (w *Worlds) Open(ctx context.Context, options SourceOptions) (Info, error) {
	// 新内容可读后才发布；退役等到旧构建结束，来源关闭再等到底层 I/O 结束。
	w.switchMu.Lock()
	defer w.switchMu.Unlock()
	source, err := OpenSource(ctx, options)
	if err != nil {
		return Info{}, err
	}
	if err = ctx.Err(); err != nil {
		source.Close()
		return Info{}, err
	}
	previous := w.view()
	current := newContent(source, previous.Info.Revision+1, w.pipeline)
	w.current.Store(current)
	previous.View.Retire()
	if previous.Minimap != nil {
		previous.Minimap.Retire()
	}
	previous.close()
	return current.Info, nil
}
func (w *Worlds) Close() {
	w.switchMu.Lock()
	defer w.switchMu.Unlock()
	current := w.view()
	current.View.Retire()
	if current.Minimap != nil {
		current.Minimap.Retire()
	}
	current.close()
}
