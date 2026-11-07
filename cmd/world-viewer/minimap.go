package main

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
	for _, action := range []string{"index", "tile", "surface"} {
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
					return content.Minimap.Tile(ctx, query.kind, minimap.Position{X: query.x, Z: query.z})
				default:
					y, err := content.Minimap.Surface(ctx, query.x, query.z)
					if err != nil {
						return nil, err
					}
					return json.Marshal(struct {
						X int32  `json:"x"`
						Z int32  `json:"z"`
						Y *int32 `json:"y"`
					}{X: query.x, Z: query.z, Y: y})
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
	// 图块坐标代表图片原点，地表坐标代表单个方块，两者按相应比例限制范围。
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
	limit := int64(29_999_999)
	if action == "tile" {
		if query.kind != "detail" && query.kind != "overview" {
			return query, fmt.Errorf("kind 需要 detail 或 overview")
		}
		size := int64(minimap.TileSize)
		if query.kind == "overview" {
			size *= 16
		}
		limit = (limit + size - 1) / size
	}
	if xErr != nil || zErr != nil || x < -limit || x > limit || z < -limit || z > limit {
		return query, fmt.Errorf("需要范围内的整数 x、z")
	}
	query.x, query.z = int32(x), int32(z)
	return query, nil
}
