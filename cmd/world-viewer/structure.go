package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"neomega-converter-core/structure"
)

// openStructureFile 首次打开时生成同路径同名存档，再交给现有 Java/Bedrock 来源。
// 同名目录已是存档时直接复用，既避免重复转换，也保留用户在存档中的编辑。
func openStructureFile(ctx context.Context, options SourceOptions) (*Source, error) {
	absolute, err := filepath.Abs(options.File)
	if err != nil {
		return nil, err
	}
	directory := structure.WorldDirectory(absolute)
	var conversionSeconds *float64
	reused := false
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) || (err == nil && len(entries) == 0) {
		started := time.Now()
		if _, err = structure.ConvertFile(ctx, absolute, directory, structure.Options{Progress: options.Progress}); err != nil {
			return nil, err
		}
		seconds := time.Since(started).Seconds()
		conversionSeconds = &seconds
	} else if err != nil {
		return nil, err
	} else {
		reused = true
	}
	converted, err := sourceAtPath(directory)
	if err != nil {
		return nil, err
	}
	converted.Position = options.Position
	converted.SkipBedrockFix = options.SkipBedrockFix
	opened, err := OpenSource(ctx, converted)
	if err != nil {
		return nil, err
	}
	opened.Info.ConversionSeconds = conversionSeconds
	opened.Info.ReusedStructure = reused
	opened.Info.Path = absolute
	opened.Info.Kind = structure.FileKind(absolute)
	return opened, nil
}
