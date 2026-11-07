package main

import (
	"context"
	"fmt"
	"neomega-converter-core/structure"
	"os"
	"path/filepath"

	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer/minimap"
	"neomega-viewer/source"
	"neomega-viewer/viewhttp"
)

type Info struct {
	// 场景描述复用页面契约，存档选择器需要的路径和格式由程序补充。
	viewhttp.Description
	Minimap           bool     `json:"minimap"`
	Version           string   `json:"version"`
	Path              string   `json:"path"`
	Kind              string   `json:"kind"`
	Dimension         string   `json:"dimension"`
	ConversionSeconds *float64 `json:"conversionSeconds,omitempty"`
	ReusedStructure   bool     `json:"reusedStructure,omitempty"`
	// 非空表示当前显示默认场景，页面据此换上线框、天空和取景。
	Backdrop *Backdrop `json:"backdrop,omitempty"`
}

type SourceOptions struct {
	StructureMemoryMiB int
	StructureWorkers   int
	// 同一参数在识别格式后解释：Java 维度是相对目录，Bedrock 维度是编号。
	Directory      string
	Pack           string
	File           string
	Kind           string
	Dimension      string
	Position       *[3]float64
	SkipBedrockFix bool
	Progress       func(structure.Progress)
}

type Source struct {
	// 打开方绑定 reader 工厂与关闭动作，列构建不需要知道原生句柄的类型。
	Info    Info
	reader  func() dispatcher.SectionSource
	minimap *minimap.Options
	close   func()
}

// OpenSource 按选项打开一种来源：结构文件先转成原生存档，其余直接打开。
// Directory、Pack、File 三选一，都不给时按 Java 存档目录处理。
func OpenSource(ctx context.Context, options SourceOptions) (*Source, error) {
	// 命令行和页面共用格式识别；结构文件先生成原生存档再打开。
	sources := 0
	if options.Directory != "" {
		sources++
	}
	if options.Pack != "" {
		sources++
	}
	if options.File != "" {
		sources++
	}
	if sources > 1 {
		return nil, fmt.Errorf("只能指定一种来源：存档目录、PackFS 或结构文件")
	}
	if options.File != "" {
		return openStructureFile(ctx, options)
	}
	if options.Directory != "" && isBedrockDirectory(options.Directory) {
		return openBedrockSource(options)
	}
	return openJavaSource(options)
}

func isBedrockDirectory(directory string) bool {
	// 有些导出的 Bedrock 世界只留下 db，不能以 level.dat 是否存在识别格式。
	stat, err := os.Stat(filepath.Join(directory, "db"))
	return err == nil && stat.IsDir()
}

// Reader 为一次列构建申请独立 reader；来源没有 reader 时返回空场景。
func (s *Source) Reader() dispatcher.SectionSource {
	if s.reader == nil {
		return source.MemorySections{}
	}
	return s.reader()
}

func (s *Source) Close() {
	// 各格式的关闭动作先等原始读取结束，再释放存档句柄。
	s.close()
}
