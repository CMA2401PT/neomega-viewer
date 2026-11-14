package worldview

import (
	"fmt"
	"neomega-converter-core/structure"
	"os"
	"path/filepath"
	"strings"
)

// SourceAtPath 把用户给的路径解释成一种来源：存档目录、level.dat、PackFS 或结构文件。
func SourceAtPath(name string) (SourceOptions, error) {
	// 从终端“拷贝路径”出来的字符串会带引号。
	name = strings.TrimSpace(name)
	if len(name) >= 2 && name[0] == '"' && name[len(name)-1] == '"' {
		name = name[1 : len(name)-1]
	}
	if name == "" {
		return SourceOptions{}, fmt.Errorf("请选择存档目录、level.dat、PackFS 或结构文件")
	}
	absolute, err := filepath.Abs(name)
	if err != nil {
		return SourceOptions{}, err
	}
	stat, err := os.Stat(absolute)
	if err != nil {
		return SourceOptions{}, err
	}
	if stat.IsDir() {
		if isBedrockDirectory(absolute) {
			return SourceOptions{Directory: absolute}, nil
		}
		for _, child := range []string{"level.dat", "region", "dimensions"} {
			if _, err := os.Stat(filepath.Join(absolute, child)); err == nil {
				return SourceOptions{Directory: absolute}, nil
			}
		}
		return SourceOptions{}, fmt.Errorf("此目录不是存档：请选择包含 level.dat、region 或 db 子目录的世界目录")
	}
	// 选 level.dat 等于选它所在的存档目录。
	if strings.EqualFold(filepath.Base(absolute), "level.dat") {
		return SourceOptions{Directory: filepath.Dir(absolute)}, nil
	}
	// 空文件不当作 PackFS：packfs.Open 会把空文件当成新建容器的请求。
	if !stat.Mode().IsRegular() || stat.Size() == 0 {
		return SourceOptions{}, fmt.Errorf("请选择非空的 PackFS 或结构文件")
	}
	if kind := structure.FileKind(absolute); kind != "" {
		return SourceOptions{File: absolute, Kind: kind}, nil
	}
	return SourceOptions{Pack: absolute}, nil
}
