package javaworld

import (
	"context"
	"io/fs"

	"celler"
	"celler/java"
	"celler/java/region"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer/internal/bytecache"
)

type sourceRequest struct {
	ctx   context.Context
	pos   celler.ChunkPos
	reply chan sourceResult
}
type sourceResult struct {
	record region.Record
	err    error
}

type Source struct {
	// 一个 goroutine 独占 World 的读取与 region 缓存，worker 只共享不可变的原始字节。
	requests chan sourceRequest
	cancel   context.CancelFunc
	done     chan struct{}
}

func New(world *java.World, dim java.Dimension) *Source {
	// 调用方交出 World 的独占访问权，Close 返回后才可关闭 World 及其文件容器。
	ctx, cancel := context.WithCancel(context.Background())
	s := &Source{
		requests: make(chan sourceRequest, 8),
		cancel:   cancel,
		done:     make(chan struct{}),
	}
	go s.run(ctx, world, dim)
	return s
}

func (s *Source) Reader() dispatcher.SectionSource {
	// 每次列构建使用独立 reader，解码结果和诊断不会在 worker 之间共享。
	// 返回值同时实现 source.ColumnDiagnostics；单个 reader 由本次构建串行调用。
	return &sectionSource{
		source:  s,
		columns: map[celler.ChunkPos]*decodedColumn{},
	}
}

func (s *Source) run(ctx context.Context, world *java.World, dim java.Dimension) {
	defer close(s.done)
	cache := bytecache.New[celler.ChunkPos, region.Record](32 << 20)
	for {
		select {
		case <-ctx.Done():
			return
		case request := <-s.requests:
			if request.ctx.Err() != nil {
				continue
			}
			record, ok := cache.Get(request.pos)
			var err error
			if !ok {
				record, err = world.ReadRaw(dim, java.Terrain, request.pos)
				if err == nil {
					cache.Put(request.pos, record, len(record.Data))
				}
			}
			request.reply <- sourceResult{
				record: record,
				err:    err,
			}
		}
	}
}
func (s *Source) read(ctx context.Context, pos celler.ChunkPos) (region.Record, error) {
	if err := ctx.Err(); err != nil {
		return region.Record{}, err
	}
	reply := make(chan sourceResult, 1)
	select {
	case s.requests <- sourceRequest{
		ctx:   ctx,
		pos:   pos,
		reply: reply,
	}:
	case <-ctx.Done():
		return region.Record{}, ctx.Err()
	case <-s.done:
		return region.Record{}, fs.ErrClosed
	}
	select {
	case result := <-reply:
		return result.record, result.err
	case <-ctx.Done():
		return region.Record{}, ctx.Err()
	case <-s.done:
		return region.Record{}, fs.ErrClosed
	}
}
func (s *Source) Close() {
	// 等文件 goroutine 结束后再归还 World，取消请求本身不保证底层 I/O 已完成。
	// 此方法不关闭 World；调用方在退役相关 View 后先 Close，再关闭 World 和文件容器。
	s.cancel()
	<-s.done
}
