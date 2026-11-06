package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"neomega-converter-core/structure"
)

// openStructureFile 首次打开结构文件时先在同名目录生成原生存档，再按 Java/Bedrock 来源打开。
// 同名目录已经是存档时直接复用，既避免重复转换，也保留用户在存档中的编辑。
func openStructureFile(ctx context.Context, options SourceOptions) (*Source, error) {
	absolute, err := filepath.Abs(options.File)
	if err != nil {
		return nil, err
	}
	directory := structure.WorldDirectory(absolute)
	entries, err := os.ReadDir(directory)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	reused := len(entries) > 0
	var conversionSeconds *float64
	if !reused {
		started := time.Now()
		if _, err := structure.ConvertFile(ctx, absolute, directory, structure.Options{
			Progress:  options.Progress,
			MemoryMiB: options.StructureMemoryMiB,
			Workers:   options.StructureWorkers,
		}); err != nil {
			return nil, err
		}
		seconds := time.Since(started).Seconds()
		conversionSeconds = &seconds
	}
	world, err := sourceAtPath(directory)
	if err != nil {
		return nil, err
	}
	world.Position = options.Position
	world.SkipBedrockFix = options.SkipBedrockFix
	opened, err := OpenSource(ctx, world)
	if err != nil {
		return nil, err
	}
	opened.Info.ConversionSeconds = conversionSeconds
	opened.Info.ReusedStructure = reused
	opened.Info.Path = absolute
	opened.Info.Kind = structure.FileKind(absolute)
	return opened, nil
}
