package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"sync"

	"celler"
	"celler/bedrock"
	"neomega-converter-core/adapt"
	fixedadapt "neomega-converter/adapt"
	"neomega-viewer-core/fixer/java-postprocess"
	"neomega-viewer/source/bedrockworld"
	"neomega-viewer/viewhttp"
)

var sharedFixer = sync.OnceValues(fixer.New)

type bedrockReads struct {
	// adapt 的等待者取消后，回调仍可能在读取；计数只围住原始读取，不限制其并发。
	mu     sync.Mutex
	active sync.WaitGroup
	closed bool
	read   adapt.ReadChunk
}

func (r *bedrockReads) Read(ctx context.Context, pos celler.ChunkPos) (*bedrock.Chunk, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, fs.ErrClosed
	}
	r.active.Add(1)
	r.mu.Unlock()
	defer r.active.Done()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.read(ctx, pos)
}

func (r *bedrockReads) Close() {
	// 返回时原始读取已结束；迟到的后台回调也不能重新进入已归还的句柄。
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	r.active.Wait()
}

func openBedrockSource(options SourceOptions) (_ *Source, err error) {
	// 句柄、维度和可选修复在程序中组装，bedrockworld 只看取柱接口。
	dimension := int64(0)
	if options.Dimension != "" {
		dimension, err = strconv.ParseInt(options.Dimension, 10, 32)
		if err != nil || dimension < 0 || dimension > 2 {
			return nil, fmt.Errorf("Bedrock -dimension must be 0, 1 or 2")
		}
	}
	world, err := bedrock.Open(options.Directory, bedrock.Options{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	reads := &bedrockReads{
		read: func(ctx context.Context, pos celler.ChunkPos) (*bedrock.Chunk, error) {
			return world.ReadChunk(int32(dimension), pos)
		},
	}
	opened := &Source{
		close: func() {
			reads.Close()
			world.Close()
		},
	}
	defer func() {
		if err != nil {
			opened.Close()
		}
	}()
	position, err := bedrockPosition(world, int32(dimension))
	if err != nil {
		return nil, err
	}
	if options.Position != nil {
		position = *options.Position
	}
	absolute, err := filepath.Abs(options.Directory)
	if err != nil {
		return nil, err
	}
	opened.Info = Info{
		Description: viewhttp.Description{
			Loaded:   true,
			Name:     filepath.Base(absolute),
			Position: position,
			CanOpen:  true,
		},
		Path:      absolute,
		Kind:      "bedrock",
		Dimension: strconv.FormatInt(dimension, 10),
	}
	converted := adapt.NewConverted(reads.Read, adapt.Options{})
	var chunks adapt.ChunkSource = converted
	if !options.SkipBedrockFix {
		f, err := sharedFixer()
		if err != nil {
			return nil, err
		}
		chunks = fixedadapt.NewFixed(converted, f, adapt.Options{})
	}
	opened.reader = bedrockworld.New(chunks).Reader
	return opened, nil
}

func bedrockPosition(world *bedrock.World, dimension int32) ([3]float64, error) {
	// 出生点可缺省；回退扫描在所选维度找到第一根柱子就停止，避免启动时读整张图。
	position := [3]float64{0, 80, 0}
	if level, err := world.ReadLevel(); err == nil {
		for axis, name := range []string{"SpawnX", "SpawnY", "SpawnZ"} {
			if value, ok := level.NBT.Value[name].(int32); ok {
				position[axis] = float64(value)
			}
		}
		position[1] += 12
	}
	pos := celler.ChunkPos{
		X: int32(position[0]) >> 4,
		Z: int32(position[2]) >> 4,
	}
	if _, err := world.ReadChunk(dimension, pos); !errors.Is(err, fs.ErrNotExist) {
		return position, err
	}
	found := errors.New("found first chunk")
	err := world.ScanChunks(func(chunk *bedrock.Chunk) error {
		if chunk.Dimension != dimension {
			return nil
		}
		position[0] = float64(chunk.Pos.X)*16 + 8
		position[2] = float64(chunk.Pos.Z)*16 + 8
		highest := int32(-129)
		for _, y := range chunk.SectionYs() {
			section, err := chunk.Section(y)
			if err != nil {
				continue
			}
			for _, layer := range section.Layers {
				for _, block := range layer.Blocks.GetUsedPalette() {
					switch block.State.NameWithNameSpace() {
					case "minecraft:air", "minecraft:cave_air", "minecraft:void_air":
					default:
						highest = max(highest, y)
					}
				}
			}
		}
		if highest >= -128 {
			position[1] = float64(highest*16 + 24)
		}
		return found
	})
	if errors.Is(err, found) {
		err = nil
	}
	return position, err
}
