package main

import (
	"fmt"
	"os"
	"path/filepath"

	"neomega-viewer/dispatcher"
	"neomega-viewer/source"
	"neomega-viewer/viewhttp"
)

type Info struct {
	// 场景描述复用页面契约，存档选择器需要的路径和格式由程序补充。
	viewhttp.Description
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Dimension string `json:"dimension"`
}

type SourceOptions struct {
	// 同一参数在识别格式后解释：Java 维度是相对目录，Bedrock 维度是编号。
	Directory      string
	Pack           string
	Dimension      string
	Position       *[3]float64
	SkipBedrockFix bool
}

type Source struct {
	// 打开方绑定 reader 工厂与关闭动作，列构建不需要知道原生句柄的类型。
	Info   Info
	reader func() dispatcher.SectionSource
	close  func()
}

func OpenSource(options SourceOptions) (*Source, error) {
	// 命令行和页面选出的目录经过同一次格式识别，PackFS 保持 Java 读取路径。
	if options.Directory != "" && options.Pack != "" {
		return nil, fmt.Errorf("choose -world or -pack")
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
