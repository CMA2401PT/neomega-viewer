package main

import (
	"context"
	"sync"
)

// Each opened world owns its cache and cancellation scope. Responses from an old
// world can never enter the next world's cache, even at identical coordinates.
type worldView struct {
	source   *Source
	revision uint64
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	cache    *byteCache[chunkKey, []byte]
}

func newWorldView(source *Source, revision uint64) *worldView {
	ctx, cancel := context.WithCancel(context.Background())
	return &worldView{source: source, revision: revision, ctx: ctx, cancel: cancel, cache: newCache[chunkKey, []byte](64 << 20)}
}
func (v *worldView) info() Info {
	info := v.source.Info
	info.Revision = v.revision
	return info
}

type Worlds struct {
	switchMu sync.Mutex
	mu       sync.RWMutex
	current  *worldView
}

func NewWorlds(source *Source) *Worlds { return &Worlds{current: newWorldView(source, 1)} }
func (w *Worlds) view() *worldView     { w.mu.RLock(); defer w.mu.RUnlock(); return w.current }
func (w *Worlds) Open(ctx context.Context, options SourceOptions) (Info, error) {
	w.switchMu.Lock()
	defer w.switchMu.Unlock()
	source, err := OpenSource(options)
	if err != nil {
		return Info{}, err
	}
	if err = ctx.Err(); err != nil {
		source.Close()
		return Info{}, err
	}
	w.mu.Lock()
	previous := w.current
	current := newWorldView(source, previous.revision+1)
	w.current = current
	w.mu.Unlock()
	previous.cancel()
	previous.source.Close()
	return current.info(), nil
}
func (w *Worlds) Close() {
	w.switchMu.Lock()
	defer w.switchMu.Unlock()
	current := w.view()
	current.cancel()
	current.source.Close()
}
