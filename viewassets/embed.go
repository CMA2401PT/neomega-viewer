// Package viewassets 提供存档浏览应用的已构建页面。
package viewassets

import (
	"embed"
	"io/fs"
)

//go:generate node ../cmd/world-viewer/bundle.mjs

//go:embed bundle/web
var bundle embed.FS

// Web 返回可直接分发的浏览页面，普通 Go 构建无需前端工具链。
func Web() fs.FS {
	files, _ := fs.Sub(bundle, "bundle/web")
	return files
}
