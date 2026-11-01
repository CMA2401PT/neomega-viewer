package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

type fileEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
}
type directoryListing struct {
	Selected string      `json:"selected"`
	Path     string      `json:"path"`
	Parent   string      `json:"parent"`
	Roots    []fileEntry `json:"roots"`
	Entries  []fileEntry `json:"entries"`
}

// Paths are interpreted by the Go host's filepath package, including drive letters,
// backslashes and UNC shares on Windows. The browser never constructs OS paths.
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
			stat, e := os.Stat(path)
			if e != nil {
				continue
			}
			isDir = stat.IsDir()
		}
		if !isDir && !entry.Type().IsRegular() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		listing.Entries = append(listing.Entries, fileEntry{Name: entry.Name(), Path: path, Directory: isDir})
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
func sourceAtPath(name string) (SourceOptions, error) {
	// A pasted path may include the quotes used by a terminal's "copy path" action.
	name = strings.TrimSpace(name)
	if len(name) >= 2 && name[0] == '"' && name[len(name)-1] == '"' {
		name = name[1 : len(name)-1]
	}
	if name == "" {
		return SourceOptions{}, fmt.Errorf("请选择存档目录或 PackFS 文件")
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
		for _, child := range []string{"level.dat", "region", "dimensions"} {
			if _, err := os.Stat(filepath.Join(absolute, child)); err == nil {
				return SourceOptions{Directory: absolute}, nil
			}
		}
		return SourceOptions{}, fmt.Errorf("此目录不是 Java 存档：请选择包含 level.dat 或 region 的世界目录")
	}
	// Selecting level.dat is equivalent to selecting its world directory.
	if strings.EqualFold(filepath.Base(absolute), "level.dat") {
		return SourceOptions{Directory: filepath.Dir(absolute)}, nil
	}
	// PackFS owns format validation. Reject empty files because packfs.Open
	// treats them as a request to create a new container.
	if !stat.Mode().IsRegular() || stat.Size() == 0 {
		return SourceOptions{}, fmt.Errorf("请选择已有的 PackFS 存档文件")
	}
	return SourceOptions{Pack: absolute}, nil
}
