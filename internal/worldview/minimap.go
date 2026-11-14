package worldview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"neomega-viewer/minimap"
)

func registerMinimap(mux *http.ServeMux, worlds *Worlds) {
	// 小地图名额独立于三维列构建，且切换存档时不额外增加并发。
	slots := make(chan struct{}, 2)
	for _, action := range []string{"index", "tile", "region"} {
		mux.HandleFunc("GET /api/minimap/"+action, func(w http.ResponseWriter, r *http.Request) {
			query, err := parseMinimapQuery(r.URL.Query(), action)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			content := worlds.view()
			if query.world != content.Info.Revision {
				http.Error(w, "存档已切换", 409)
				return
			}
			if content.Minimap == nil {
				http.Error(w, "当前场景没有小地图", 404)
				return
			}

			read := func(ctx context.Context) ([]byte, error) {
				switch action {
				case "index":
					index, err := content.Minimap.Index(ctx)
					if err != nil {
						return nil, err
					}
					return json.Marshal(index)
				case "tile":
					return content.Minimap.Tile(ctx, minimap.Position{X: query.x, Z: query.z})
				default:
					return content.Minimap.Region(ctx, minimap.Position{X: query.x, Z: query.z})
				}
			}
			var data []byte
			if action == "index" {
				data, err = read(r.Context())
			} else {
				data, err = minimap.Run(r.Context(), content.Minimap, slots, read)
			}
			if errors.Is(err, minimap.ErrRetired) || worlds.view() != content {
				http.Error(w, "存档已切换", 409)
				return
			}
			if err != nil {
				http.Error(w, err.Error(), 422)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			if action == "tile" {
				w.Header().Set("Content-Type", "image/png")
			} else if action == "region" {
				w.Header().Set("Content-Type", "application/octet-stream")
			} else {
				w.Header().Set("Content-Type", "application/json")
			}
			w.Write(data)
		})
	}
}

type minimapQuery struct {
	// HTTP 边界统一完成解析，构建只处理有确定含义的坐标。
	world uint64
	kind  string
	x, z  int32
}

func parseMinimapQuery(values url.Values, action string) (minimapQuery, error) {
	// 区域与占用图分别按 128 和 2048 格定位，负坐标采用向下取整。
	revision, err := strconv.ParseUint(values.Get("world"), 10, 64)
	if err != nil {
		return minimapQuery{}, fmt.Errorf("需要合法的 world")
	}
	query := minimapQuery{world: revision, kind: values.Get("kind")}
	if action == "index" {
		return query, nil
	}
	x, xErr := strconv.ParseInt(values.Get("x"), 10, 32)
	z, zErr := strconv.ParseInt(values.Get("z"), 10, 32)
	if action == "tile" && query.kind != "overview" {
		return query, fmt.Errorf("kind 需要 overview")
	}
	// 数据窗口会跨过玩家可移动的坐标边缘，范围外的 section 由来源返回空气。
	if xErr != nil || zErr != nil {
		return query, fmt.Errorf("需要整数 x、z")
	}
	query.x, query.z = int32(x), int32(z)
	return query, nil
}
