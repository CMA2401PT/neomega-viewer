package viewassets

import (
	"embed"
	"io/fs"
)

// Regenerate the independent frontend and copy the matching texture resources.
//go:generate node ../cmd/world-viewer/bundle.mjs

// bundle includes the built frontend, atlas metadata, textures and notices.
// Geometry tables are embedded by neomega-blocks/presets/java/v1/geometry.
//
//go:embed bundle
var bundle embed.FS

func Resources() fs.FS {
	// 两个应用共享同一套贴图、图集元数据和许可声明。
	files, _ := fs.Sub(bundle, "bundle/resources")
	return files
}

func Web() fs.FS {
	// 返回已构建的浏览页面，普通 Go 构建无需前端工具链。
	files, _ := fs.Sub(bundle, "bundle/web")
	return files
}
