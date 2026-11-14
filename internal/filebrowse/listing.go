package filebrowse

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"neomega-converter-core/structure"
)

// Entry 是文件浏览器的一行。Kind 决定页面用哪个图标和说明，页面不自己猜后缀。
type Entry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Kind      string `json:"kind"`
}
type Listing struct {
	Selected string  `json:"selected"`
	Path     string  `json:"path"`
	Parent   string  `json:"parent"`
	Roots    []Entry `json:"roots"`
	Entries  []Entry `json:"entries"`
}

// List 列出一个目录。路径一律由 Go 侧的 filepath 解释，包括盘符、反斜杠
// 和 Windows 的 UNC 共享，页面不构造系统路径。
func List(name string) (Listing, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Listing{}, err
	}
	if name == "" {
		name = home
	}
	name = strings.Trim(strings.TrimSpace(name), "\"")
	name, err = filepath.Abs(name)
	if err != nil {
		return Listing{}, err
	}
	// 选中的是文件时，把它的父目录列出来，并把文件标成选中项。
	selected := ""
	if stat, e := os.Stat(name); e == nil && !stat.IsDir() {
		selected = name
		name = filepath.Dir(name)
	}
	entries, err := os.ReadDir(name)
	if err != nil {
		return Listing{}, err
	}
	listing := Listing{Selected: selected, Path: name, Parent: filepath.Dir(name), Entries: []Entry{}, Roots: []Entry{{Name: "主目录", Path: home, Directory: true}}}
	if runtime.GOOS == "windows" {
		for drive := 'A'; drive <= 'Z'; drive++ {
			root := string(drive) + ":\\"
			if stat, e := os.Stat(root); e == nil && stat.IsDir() {
				listing.Roots = append(listing.Roots, Entry{Name: root, Path: root, Directory: true})
			}
		}
	} else {
		listing.Roots = append(listing.Roots, Entry{Name: "/", Path: "/", Directory: true})
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
		listing.Entries = append(listing.Entries, Entry{
			Name:      entry.Name(),
			Path:      path,
			Directory: isDir,
			Kind:      entryKind(isDir, entry.Name(), path),
		})
	}
	slices.SortFunc(listing.Entries, func(a, b Entry) int {
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
