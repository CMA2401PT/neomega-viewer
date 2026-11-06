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
	"neomega-viewer/source/javaworld"
	"neomega-viewer/viewhttp"
)

func openJavaSource(options SourceOptions) (_ *Source, err error) {
	// 先完成路径打开和落点探测，再把 World 的独占读取权交给 javaworld。
	opened := &Source{}
	var world *java.World
	var reader *javaworld.Source
	var closeContainer func()
	opened.close = func() {
		if reader != nil {
			reader.Close()
		}
		if world != nil {
			world.Close()
		}
		if closeContainer != nil {
			closeContainer()
		}
	}
	defer func() {
		if err != nil {
			opened.Close()
		}
	}()
	opened.Info = Info{
		Description: viewhttp.Description{
			Position: [3]float64{0, 80, 0},
			CanOpen:  true,
		},
		Dimension: options.Dimension,
	}
	dimension := java.Dimension(options.Dimension)
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
			closeContainer = func() { container.Close() }
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
		if dimension == "" {
			dimension = java.Overworld
			if _, e := fs.Stat(world.Files, "region"); errors.Is(e, fs.ErrNotExist) {
				if _, e = fs.Stat(world.Files, "dimensions/minecraft/overworld/region"); e == nil {
					dimension = "dimensions/minecraft/overworld"
				}
			}
		}
		if !fs.ValidPath(string(dimension)) {
			return nil, fs.ErrInvalid
		}
		if _, e := fs.Stat(world.Files, path.Join(string(dimension), "region")); e != nil && !errors.Is(e, fs.ErrNotExist) {
			return nil, e
		}
		opened.Info.Dimension = string(dimension)
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
		if _, err := world.ReadRaw(dimension, java.Terrain, p); errors.Is(err, fs.ErrNotExist) {
			found := errors.New("found first chunk")
			err = world.Scan(dimension, java.Terrain, func(pos celler.ChunkPos) error {
				opened.Info.Position[0] = float64(pos.X)*16 + 8
				opened.Info.Position[2] = float64(pos.Z)*16 + 8
				if c, e := world.ReadChunk(dimension, pos); e == nil {
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
	if world == nil {
		applySunrise(opened)
	}
	if options.Position != nil {
		opened.Info.Position = *options.Position
	}

	if world != nil {
		reader = javaworld.New(world, dimension)
		opened.reader = reader.Reader
	}
	return opened, nil
}
