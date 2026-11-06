package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"neomega-converter-core/structure"
)

// fileEntry 是文件浏览器的一行。Kind 决定页面用哪个图标和说明，页面不自己猜后缀。
type fileEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Kind      string `json:"kind"`
}
type directoryListing struct {
	Selected string      `json:"selected"`
	Path     string      `json:"path"`
	Parent   string      `json:"parent"`
	Roots    []fileEntry `json:"roots"`
	Entries  []fileEntry `json:"entries"`
}

// browseDirectory 列出一个目录。路径一律由 Go 侧的 filepath 解释，包括盘符、反斜杠
// 和 Windows 的 UNC 共享，页面不构造系统路径。
func browseDirectory(name string) (directoryListing, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return directoryListing{}, err
	}
	if name == "" {
		name = home
	}
	name = strings.Trim(strings.TrimSpace(name), "\"")
	name, err = filepath.Abs(name)
	if err != nil {
		return directoryListing{}, err
	}
	// 选中的是文件时，把它的父目录列出来，并把文件标成选中项。
	selected := ""
	if stat, e := os.Stat(name); e == nil && !stat.IsDir() {
		selected = name
		name = filepath.Dir(name)
	}
	entries, err := os.ReadDir(name)
	if err != nil {
		return directoryListing{}, err
	}
	listing := directoryListing{Selected: selected, Path: name, Parent: filepath.Dir(name), Entries: []fileEntry{}, Roots: []fileEntry{{Name: "主目录", Path: home, Directory: true}}}
	if runtime.GOOS == "windows" {
		for drive := 'A'; drive <= 'Z'; drive++ {
			root := string(drive) + ":\\"
			if stat, e := os.Stat(root); e == nil && stat.IsDir() {
				listing.Roots = append(listing.Roots, fileEntry{Name: root, Path: root, Directory: true})
			}
		}
	} else {
		listing.Roots = append(listing.Roots, fileEntry{Name: "/", Path: "/", Directory: true})
	}
	for _, entry := range entries {
		path := filepath.Join(name, entry.Name())
		isDir := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			// 符号链接按目标判断是不是目录；断掉的链接直接跳过。
			stat, e := os.Stat(path)
			if e != nil {
				continue
			}
			isDir = stat.IsDir()
		}
		if !isDir && !entry.Type().IsRegular() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		listing.Entries = append(listing.Entries, fileEntry{
			Name:      entry.Name(),
			Path:      path,
			Directory: isDir,
			Kind:      entryKind(isDir, entry.Name(), path),
		})
	}
	slices.SortFunc(listing.Entries, func(a, b fileEntry) int {
		if a.Directory != b.Directory {
			if a.Directory {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return listing, nil
}

// entryKind 给目录条目分类：目录、存档入口、PackFS、结构文件，其余是未识别后缀。
func entryKind(isDir bool, name, path string) string {
	kind := structure.FileKind(path)
	switch {
	case isDir:
		return "folder"
	case strings.EqualFold(name, "level.dat"):
		return "level"
	case kind != "":
		return kind
	case strings.EqualFold(filepath.Ext(path), ".pack"), strings.EqualFold(filepath.Ext(path), ".packfs"):
		return "pack"
	default:
		return "unknown"
	}
}

// sourceAtPath 把用户给的路径解释成一种来源：存档目录、level.dat、PackFS 或结构文件。
func sourceAtPath(name string) (SourceOptions, error) {
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
