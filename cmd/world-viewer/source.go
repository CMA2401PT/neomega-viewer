package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"celler"
	"celler/java"
	"celler/java/region"
	packfs "neomega-packfs"
)

type Info struct {
	Revision        uint64     `json:"revision"`
	Path            string     `json:"path"`
	Kind            string     `json:"kind"`
	Loaded          bool       `json:"loaded"`
	Name            string     `json:"name"`
	Dimension       string     `json:"dimension"`
	Position        [3]float64 `json:"position"`
	ResourceVersion string     `json:"resourceVersion"`
}
type SourceOptions struct {
	Directory string
	Pack      string
	Dimension java.Dimension
	Position  *[3]float64
}
type sourceRequest struct {
	ctx   context.Context
	pos   celler.ChunkPos
	reply chan sourceResult
}
type sourceResult struct {
	record region.Record
	err    error
}

// Source owns every world and PackFS handle on one goroutine. Returned compressed
// bytes are immutable; workers decode independent documents from them.
type Source struct {
	Info     Info
	requests chan sourceRequest
	cancel   context.CancelFunc
	done     chan struct{}
}

func OpenSource(options SourceOptions) (*Source, error) {
	if options.Directory != "" && options.Pack != "" {
		return nil, fmt.Errorf("choose -world or -pack")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Source{requests: make(chan sourceRequest, 8), cancel: cancel, done: make(chan struct{})}
	ready := make(chan error, 1)
	go s.run(ctx, options, ready)
	if err := <-ready; err != nil {
		cancel()
		<-s.done
		return nil, err
	}
	return s, nil
}
func (s *Source) run(ctx context.Context, options SourceOptions, ready chan<- error) {
	defer close(s.done)
	s.Info = Info{Dimension: string(options.Dimension), Position: [3]float64{0, 80, 0}}
	var world *java.World
	var container *packfs.PackFS
	var err error
	switch {
	case options.Pack != "":
		var stat os.FileInfo
		if stat, err = os.Stat(options.Pack); err == nil {
			if !stat.Mode().IsRegular() || stat.Size() == 0 {
				err = fmt.Errorf("请选择已有的 PackFS 存档文件")
			} else {
				container, err = packfs.Open(options.Pack)
			}
		}
		if err == nil {
			defer container.Close()
			world, err = java.Open(container, java.Options{ReadOnly: true, MaxOpenRegions: 16})
		}
		s.Info.Name = filepath.Base(options.Pack)
		s.Info.Path, _ = filepath.Abs(options.Pack)
		s.Info.Kind = "pack"
	case options.Directory != "":
		world, err = java.OpenDirectory(options.Directory, java.Options{ReadOnly: true, MaxOpenRegions: 16})
		s.Info.Name = filepath.Base(options.Directory)
		s.Info.Path, _ = filepath.Abs(options.Directory)
		s.Info.Kind = "directory"
	}
	if err != nil {
		ready <- err
		return
	}
	if world != nil {
		defer world.Close()
		if options.Dimension == "" {
			options.Dimension = java.Overworld
			if _, e := fs.Stat(world.Files, "region"); errors.Is(e, fs.ErrNotExist) {
				if _, e = fs.Stat(world.Files, "dimensions/minecraft/overworld/region"); e == nil {
					options.Dimension = "dimensions/minecraft/overworld"
				}
			}
		}
		if !fs.ValidPath(string(options.Dimension)) {
			ready <- fs.ErrInvalid
			return
		}
		if _, e := fs.Stat(world.Files, path.Join(string(options.Dimension), "region")); e != nil && !errors.Is(e, fs.ErrNotExist) {
			ready <- e
			return
		}
		s.Info.Dimension = string(options.Dimension)
		s.Info.Loaded = true
		if level, err := world.ReadLevel(); err == nil {
			if data, ok := level.Value["Data"].(map[string]any); ok {
				for axis, key := range []string{"SpawnX", "SpawnY", "SpawnZ"} {
					if value, ok := data[key].(int32); ok {
						s.Info.Position[axis] = float64(value)
					}
				}
				s.Info.Position[1] += 12
			}
		}
		// Find one occupied slot if the selected dimension has no chunk at spawn.
		p := celler.ChunkPos{X: int32(s.Info.Position[0]) >> 4, Z: int32(s.Info.Position[2]) >> 4}
		if _, err := world.ReadRaw(options.Dimension, java.Terrain, p); errors.Is(err, fs.ErrNotExist) {
			found := errors.New("found first chunk")
			err = world.Scan(options.Dimension, java.Terrain, func(pos celler.ChunkPos) error {
				s.Info.Position[0] = float64(pos.X)*16 + 8
				s.Info.Position[2] = float64(pos.Z)*16 + 8
				if c, e := world.ReadChunk(options.Dimension, pos); e == nil {
					if ys, e := c.SectionYs(); e == nil {
						highest := int32(-129)
						for _, y := range ys {
							section, e := c.Section(y)
							if e != nil || section.Blocks == nil {
								continue
							}
							for _, block := range section.Blocks.GetUsedPalette() {
								switch block.NameWithNameSpace() {
								case "minecraft:air", "minecraft:cave_air", "minecraft:void_air":
								default:
									highest = max(highest, y)
								}
							}
						}
						if highest >= -128 {
							s.Info.Position[1] = float64(highest*16 + 24)
						}
					}
				}
				return found
			})
			if err != nil && !errors.Is(err, found) {
				ready <- err
				return
			}
		} else if err != nil {
			ready <- err
			return
		}
	}
	if options.Position != nil {
		s.Info.Position = *options.Position
	}
	ready <- nil
	cache := newCache[celler.ChunkPos, region.Record](32 << 20)
	for {
		select {
		case <-ctx.Done():
			return
		case request := <-s.requests:
			if request.ctx.Err() != nil {
				continue
			}
			record, ok := cache.get(request.pos)
			var err error
			if !ok {
				if world == nil {
					err = fs.ErrNotExist
				} else {
					record, err = world.ReadRaw(options.Dimension, java.Terrain, request.pos)
				}
				if err == nil {
					cache.put(request.pos, record, len(record.Data))
				}
			}
			request.reply <- sourceResult{record: record, err: err}
		}
	}
}
func (s *Source) Read(ctx context.Context, pos celler.ChunkPos) (region.Record, error) {
	reply := make(chan sourceResult, 1)
	select {
	case s.requests <- sourceRequest{ctx: ctx, pos: pos, reply: reply}:
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
func (s *Source) Close() { s.cancel(); <-s.done }
