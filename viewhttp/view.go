package viewhttp

import (
	"context"
	"sync"

	"neomega-viewer/column"
	"neomega-viewer/internal/bytecache"
)

type View struct {
	// 固定一代内容与构建闭包，取消和缓存始终跟随同一代，来源由组装方持有。
	revision uint64
	ctx      context.Context
	cancel   context.CancelFunc
	build    func(context.Context, column.Key) ([]byte, error)
	mu       sync.Mutex
	retired  bool
	active   sync.WaitGroup
	cache    *bytecache.Cache[column.Key, []byte]
}

func NewView(revision uint64, build func(context.Context, column.Key) ([]byte, error)) *View {
	// build 可以并发调用；它捕获的内容在 Retire 返回前必须保持可读。
	// revision 由组装方分配，每次发布内容都使用新的值；返回字节在缓存期间只读。
	ctx, cancel := context.WithCancel(context.Background())
	return &View{
		revision: revision,
		ctx:      ctx,
		cancel:   cancel,
		build:    build,
		cache:    bytecache.New[column.Key, []byte](64 << 20),
	}
}

func (v *View) Retire() {
	// 禁止新请求进入后再等待，避免取消已发出但来源仍在读取时就关闭句柄。
	// 已开始发送的响应只持有字节，不参与等待，也无法再改成 409。
	v.mu.Lock()
	v.retired = true
	v.cancel()
	v.mu.Unlock()
	v.active.Wait()
	v.mu.Lock()
	v.cache = nil
	v.mu.Unlock()
}

func (v *View) enter() bool {
	// 登记与退役共用锁，使 Wait 不会漏掉刚开始的构建。
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.retired {
		return false
	}
	v.active.Add(1)
	return true
}
