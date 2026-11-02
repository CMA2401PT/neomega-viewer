package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"celler"
	"celler/java"
	packfs "neomega-packfs"
	"neomega-viewer/dispatcher"
	"neomega-viewer/source"
	"neomega-viewer/source/javaworld"
)

type Info struct {
	// 浏览页面需要的存档描述，与对应 View 一起发布。
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
	// 路径选择和初始落点属于程序，Java 来源只接收已打开的世界与维度。
	Directory string
	Pack      string
	Dimension java.Dimension
	Position  *[3]float64
}
type Source struct {
	// 打开过程和落点探测在移交句柄前完成，关闭顺序保证文件访问已经停止。
	Info           Info
	world          *java.World
	reader         *javaworld.Source
	closeContainer func()
}

func OpenSource(options SourceOptions) (_ *Source, err error) {
	// 先完成路径打开和落点探测，再把 World 的独占读取权交给 javaworld。
	if options.Directory != "" && options.Pack != "" {
		return nil, fmt.Errorf("choose -world or -pack")
	}
	opened := &Source{}
	defer func() {
		if err != nil {
			opened.Close()
		}
	}()
	opened.Info = Info{
		Dimension: string(options.Dimension),
		Position:  [3]float64{0, 80, 0},
	}
	var world *java.World
	var container *packfs.PackFS
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
			opened.closeContainer = func() { container.Close() }
			world, err = java.Open(container, java.Options{
				ReadOnly:       true,
				MaxOpenRegions: 16,
			})
		}
		opened.Info.Name = filepath.Base(options.Pack)
		opened.Info.Path, _ = filepath.Abs(options.Pack)
		opened.Info.Kind = "pack"
	case options.Directory != "":
		world, err = java.OpenDirectory(options.Directory, java.Options{
			ReadOnly:       true,
			MaxOpenRegions: 16,
		})
		opened.Info.Name = filepath.Base(options.Directory)
		opened.Info.Path, _ = filepath.Abs(options.Directory)
		opened.Info.Kind = "directory"
	}
	if err != nil {
		return nil, err
	}
	if world != nil {
		opened.world = world
		if options.Dimension == "" {
			options.Dimension = java.Overworld
			if _, e := fs.Stat(world.Files, "region"); errors.Is(e, fs.ErrNotExist) {
				if _, e = fs.Stat(world.Files, "dimensions/minecraft/overworld/region"); e == nil {
					options.Dimension = "dimensions/minecraft/overworld"
				}
			}
		}
		if !fs.ValidPath(string(options.Dimension)) {
			return nil, fs.ErrInvalid
		}
		if _, e := fs.Stat(world.Files, path.Join(string(options.Dimension), "region")); e != nil && !errors.Is(e, fs.ErrNotExist) {
			return nil, e
		}
		opened.Info.Dimension = string(options.Dimension)
		opened.Info.Loaded = true
		if level, err := world.ReadLevel(); err == nil {
			if data, ok := level.Value["Data"].(map[string]any); ok {
				for axis, key := range []string{"SpawnX", "SpawnY", "SpawnZ"} {
					if value, ok := data[key].(int32); ok {
						opened.Info.Position[axis] = float64(value)
					}
				}
				opened.Info.Position[1] += 12
			}
		}
		// Find one occupied slot if the selected dimension has no chunk at spawn.
		p := celler.ChunkPos{
			X: int32(opened.Info.Position[0]) >> 4,
			Z: int32(opened.Info.Position[2]) >> 4,
		}
		if _, err := world.ReadRaw(options.Dimension, java.Terrain, p); errors.Is(err, fs.ErrNotExist) {
			found := errors.New("found first chunk")
			err = world.Scan(options.Dimension, java.Terrain, func(pos celler.ChunkPos) error {
				opened.Info.Position[0] = float64(pos.X)*16 + 8
				opened.Info.Position[2] = float64(pos.Z)*16 + 8
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
							opened.Info.Position[1] = float64(highest*16 + 24)
						}
					}
				}
				return found
			})
			if err != nil && !errors.Is(err, found) {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		}
	}
	if options.Position != nil {
		opened.Info.Position = *options.Position
	}

	if world != nil {
		opened.reader = javaworld.New(world, options.Dimension)
	}
	return opened, nil
}

func (s *Source) Reader() dispatcher.SectionSource {
	if s.reader == nil {
		return source.MemorySections{}
	}
	return s.reader.Reader()
}

func (s *Source) Close() {
	// 原始记录读取可能比请求取消更晚结束，先等待 reader，再关闭底层句柄。
	if s.reader != nil {
		s.reader.Close()
	}
	if s.world != nil {
		s.world.Close()
	}
	if s.closeContainer != nil {
		s.closeContainer()
	}
}
