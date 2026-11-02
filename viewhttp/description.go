package viewhttp

import (
	"encoding/json"
	"net/http"
)

type Description struct {
	// 配套页面启动所需的场景描述，与同代 View 使用相同 revision。
	// CanOpen 表示应用是否提供内容选择入口；路径和来源格式由应用自行扩展。
	Revision        uint64     `json:"revision"`
	Loaded          bool       `json:"loaded"`
	Name            string     `json:"name"`
	Position        [3]float64 `json:"position"`
	ResourceVersion string     `json:"resourceVersion"`
	CanOpen         bool       `json:"canOpen"`
}

func DescriptionHandler(description Description) http.Handler {
	// 固定场景可以直接提供启动描述；路由由应用注册，以便沿用已有页面地址。
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(description)
	})
}
