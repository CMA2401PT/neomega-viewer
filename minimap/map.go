package minimap

import (
	"bytes"
	"context"
	"errors"
	"image/color"
	"sync"

	"celler"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer/internal/bytecache"
)

var ErrRetired = errors.New("小地图已退役")

type Position struct {
	// 坐标以图块为单位；负数采用向下取整，与页面一致。
	X int32 `json:"x"`
	Z int32 `json:"z"`
}

type Index struct {
	// 索引就绪后页面跳过空白图块；完整区块集合留在服务端用于占用图。
	Regions  []Position `json:"regions"`
	Overview []Position `json:"overview"`
}

type Options struct {
	// Reader 每次创建独立解码器，Chunks 只扫描坐标，不解码方块。
	Terrain Terrain
	Reader  func() dispatcher.SectionSource
	Chunks  func(context.Context) ([]celler.ChunkPos, error)
}

type tileKey struct {
	Kind string
	Position
}

type Map struct {
	// 取消、在途读取和缓存同属一代；Retire 返回后组装方才可以关闭来源。
	options   Options
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	retired   bool
	active    sync.WaitGroup
	cache     *bytecache.Cache[tileKey, []byte]
	indexDone chan struct{}
	indexErr  error
	index     *Index
	chunks    map[celler.ChunkPos]bool
}

func New(options Options) *Map {
	// 并发名额由 HTTP 层跨代共享，地图只持有本代的数据。
	ctx, cancel := context.WithCancel(context.Background())
	return &Map{options: options, ctx: ctx, cancel: cancel, cache: bytecache.New[tileKey, []byte](64 << 20)}
}

func (m *Map) Retire() {
	// 登记和禁止新任务共用锁，取消后等待所有来源访问归还。
	m.mu.Lock()
	m.retired = true
	m.cancel()
	m.mu.Unlock()
	m.active.Wait()
}

func Run[T any](ctx context.Context, m *Map, slots chan struct{}, work func(context.Context) (T, error)) (T, error) {
	// 请求取消与代次取消合并，排队中的任务也参加退役等待。
	var zero T
	m.mu.Lock()
	if m.retired {
		m.mu.Unlock()
		return zero, ErrRetired
	}
	m.active.Add(1)
	m.mu.Unlock()
	defer m.active.Done()
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(m.ctx, cancel)
	defer stop()
	defer cancel()
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		if m.ctx.Err() != nil {
			return zero, ErrRetired
		}
		return zero, ctx.Err()
	}
	defer func() { <-slots }()
	result, err := work(ctx)
	if m.ctx.Err() != nil {
		return zero, ErrRetired
	}
	if ctx.Err() != nil {
		return zero, ctx.Err()
	}
	return result, err
}

func (m *Map) Index(ctx context.Context) (*Index, error) {
	// 扫描属于地图这一代，请求取消只停止等待，后来的请求仍能取得同一份结果。
	m.mu.Lock()
	if m.retired {
		m.mu.Unlock()
		return nil, ErrRetired
	}
	if m.indexDone == nil {
		m.indexDone = make(chan struct{})
		m.active.Add(1)
		go m.scanIndex()
	}
	done := m.indexDone
	m.mu.Unlock()
	select {
	case <-done:
		if m.ctx.Err() != nil {
			return nil, ErrRetired
		}
		return m.index, m.indexErr
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.ctx.Done():
		return nil, ErrRetired
	}
}

func (m *Map) scanIndex() {
	// 发布结果前完成全部索引，关闭通知通道后所有等待者都可以只读共享。
	defer m.active.Done()
	defer close(m.indexDone)
	positions, err := m.options.Chunks(m.ctx)
	if err != nil {
		m.indexErr = err
		return
	}
	index := &Index{Regions: []Position{}, Overview: []Position{}}
	chunks := make(map[celler.ChunkPos]bool, len(positions))
	regions, overview := map[Position]bool{}, map[Position]bool{}
	for _, pos := range positions {
		chunks[pos] = true
		d := Position{X: pos.X >> 3, Z: pos.Z >> 3}
		o := Position{X: pos.X >> 7, Z: pos.Z >> 7}
		if !regions[d] {
			regions[d] = true
			index.Regions = append(index.Regions, d)
		}
		if !overview[o] {
			overview[o] = true
			index.Overview = append(index.Overview, o)
		}
	}
	m.chunks, m.index = chunks, index
}

func (m *Map) Tile(ctx context.Context, pos Position) ([]byte, error) {
	// 索引只保存区块坐标，占用图不读取方块或材质。
	return m.cached(ctx, tileKey{Kind: "overview", Position: pos}, func() ([]byte, error) {
		if _, err := m.Index(ctx); err != nil {
			return nil, err
		}
		pixels := make([]color.NRGBA, TileSize*TileSize)
		for z := range TileSize {
			for x := range TileSize {
				if m.chunks[celler.ChunkPos{X: pos.X*TileSize + int32(x), Z: pos.Z*TileSize + int32(z)}] {
					pixels[z*TileSize+x] = color.NRGBA{R: 110, G: 139, B: 116, A: 255}
				}
			}
		}
		return encode(pixels)
	})
}

func (m *Map) Region(ctx context.Context, pos Position) ([]byte, error) {
	// Map 固定绑定一个存档代次和维度，区域坐标只在这份缓存内有意义。
	return m.cached(ctx, tileKey{Kind: "region", Position: pos}, func() ([]byte, error) {
		volume, err := m.options.Terrain.Region(ctx, m.options.Reader, pos)
		if err != nil {
			return nil, err
		}
		var output bytes.Buffer
		err = volume.Write(&output)
		return output.Bytes(), err
	})
}

func (m *Map) cached(ctx context.Context, key tileKey, build func() ([]byte, error)) ([]byte, error) {
	// 锁只保护缓存；不同区域可以同时读取，取消的构建不发布结果。
	m.mu.Lock()
	data, ok := m.cache.Get(key)
	m.mu.Unlock()
	if ok {
		return data, nil
	}
	data, err := build()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.cache.Put(key, data, len(data))
	m.mu.Unlock()
	return data, nil
}
