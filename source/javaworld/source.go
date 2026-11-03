package javaworld

import (
	"context"
	"errors"
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

// columnRecords 分别保留 Terrain 和 Entities；任一缺失都不掩盖另一份记录。
type columnRecords [2]*region.Record

type sourceResult struct {
	record columnRecords
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
	cache := bytecache.New[celler.ChunkPos, columnRecords](32 << 20)
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
				for index, kind := range []java.RegionKind{java.Terrain, java.Entities} {
					value, readErr := world.ReadRaw(dim, kind, request.pos)
					if errors.Is(readErr, fs.ErrNotExist) {
						continue
					}
					if readErr != nil {
						err = readErr
						break
					}
					record[index] = &value
				}
				if err == nil {
					size := 0
					for _, value := range record {
						if value != nil {
							size += len(value.Data)
						}
					}
					cache.Put(request.pos, record, size)
				}
			}
			request.reply <- sourceResult{
				record: record,
				err:    err,
			}
		}
	}
}
func (s *Source) read(ctx context.Context, pos celler.ChunkPos) (columnRecords, error) {
	if err := ctx.Err(); err != nil {
		return columnRecords{}, err
	}
	reply := make(chan sourceResult, 1)
	select {
	case s.requests <- sourceRequest{
		ctx:   ctx,
		pos:   pos,
		reply: reply,
	}:
	case <-ctx.Done():
		return columnRecords{}, ctx.Err()
	case <-s.done:
		return columnRecords{}, fs.ErrClosed
	}
	select {
	case result := <-reply:
		return result.record, result.err
	case <-ctx.Done():
		return columnRecords{}, ctx.Err()
	case <-s.done:
		return columnRecords{}, fs.ErrClosed
	}
}
func (s *Source) Close() {
	// 等文件 goroutine 结束后再归还 World，取消请求本身不保证底层 I/O 已完成。
	// 此方法不关闭 World；调用方在退役相关 View 后先 Close，再关闭 World 和文件容器。
	s.cancel()
	<-s.done
}
