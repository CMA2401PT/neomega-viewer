package decoratedpot

import (
	"math"
	"strings"

	blockgeometry "neomega-blocks/geometry"
	"neomega-blocks/geometry/material"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer/blockactor"
	"neomega-viewer/mesher"
	"neomega-viewer/mesher/geometry"
	"neomega-viewer/splitlayer"
)

type SurfaceLookup func(texture string) (material.Surface, error)

// 贴图名带 minecraft: 命名空间，查询结果包含配套图集区域和材质属性。

type Handler struct {
	models   *mesher.Mesher[uint32]
	surfaces SurfaceLookup
}

func New(tables *blockgeometry.Tables[uint32], surfaces SurfaceLookup) *Handler {
	return &Handler{models: mesher.NewMesher(tables), surfaces: surfaces}
}

func (h *Handler) BlockNames() []string {
	return []string{"minecraft:decorated_pot"}
}

func decorations(snbt string) [4]string {
	// 目标版本把 sherds 存成按面命名的 compound；1.20 起的早期存档是四项列表，一并接受，格式见 NBT.md。
	var root map[string]any
	var items [4]string
	if err := nbt.DecodeFromSNBT(snbt, &root, nbt.OptDecodeFromSNBT{}); err != nil {
		return items
	}
	switch sherds := root["sherds"].(type) {
	case []any:
		for i := range min(len(sherds), len(items)) {
			items[i], _ = sherds[i].(string)
		}
	case map[string]any:
		for i, side := range []string{"back", "left", "right", "front"} {
			item, _ := sherds[side].(map[string]any)
			items[i], _ = item["id"].(string)
		}
	}
	return items
}

func texture(item string) string {
	name := "decorated_pot_side"
	if sherd, found := strings.CutPrefix(item, "minecraft:"); found && strings.HasSuffix(sherd, "_pottery_sherd") {
		name = strings.TrimSuffix(sherd, "_pottery_sherd") + "_pottery_pattern"
	}
	return "minecraft:entity/decorated_pot/" + name
}

func (h *Handler) Build(input blockactor.Input) (geometry.Mesh, error) {
	// 表已经按 facing 旋转，复用它可保留 Neck、Head 与顶底面的精确形状和材质。
	mesh, err := h.models.MeshBlock(splitlayer.SplitBlock(input.State)[0])
	if err != nil {
		return geometry.Mesh{}, err
	}
	items := decorations(input.SNBT)
	var surfaces [4]material.Surface
	for i, item := range items {
		surfaces[i], err = h.surfaces(texture(item))
		if err != nil {
			return geometry.Mesh{}, err
		}
	}
	facing, _ := input.State.States().Get("facing")
	front := map[string]geometry.Vec3{
		"north": {0, 0, 1}, "east": {-1, 0, 0},
		"south": {0, 0, -1}, "west": {1, 0, 0},
	}[facing.(string)]
	// Java 的原始 front 在 +Z，绕 Y 转 180°-toYRot；north 时左右分别在 -X、+X。
	normals := [4]geometry.Vec3{
		{-front[0], 0, -front[2]},
		{-front[2], 0, front[0]},
		{front[2], 0, -front[0]},
		front,
	}
	for i := range mesh.Quads {
		quad := &mesh.Quads[i]
		// Body 的侧面从 y=0 到 y=1；上方的 Neck 与 Head 不参与换贴图。
		lo, hi := quad.Positions[0][1], quad.Positions[0][1]
		for _, p := range quad.Positions {
			lo, hi = min(lo, p[1]), max(hi, p[1])
		}
		if math.Abs(lo) > 1e-9 || math.Abs(hi-1) > 1e-9 {
			continue
		}
		for side, normal := range normals {
			if dot(quad.Normal, normal) < .999 {
				continue
			}
			quad.Surface = surfaces[side]
			// 原版侧面 texOffs(1,0)，在 16×16 纹理中取中央 14×16，保持一像素一模型单位。
			quad.UV = [4]geometry.Vec2{{1.0 / 16, 0}, {1.0 / 16, 1}, {15.0 / 16, 1}, {15.0 / 16, 0}}
		}
	}
	return mesh, nil
}

func dot(a, b geometry.Vec3) float64 {
	return a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
}
