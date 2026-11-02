package source

import (
	"context"

	"celler"
	"celler/java"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer/dispatcher"
)

type Issue struct {
	// 来源可报告整列汇总或单格问题；Position 为 nil 表示整列，否则使用世界方块坐标。
	// Count 是这条问题涉及的数量，单项问题填 1；其余可选信息留空即可。
	Reason        string
	Position      *[3]int32
	Blockstate    string
	Count         int
	BlockEntityID string
	Detail        string
}

type ColumnDiagnostics interface {
	// 在该列的 Section 读取完成后查询整列诊断，条目不随请求的高度窗口裁剪。
	// 同一代内容的条目及计数应保持一致，供前端消除重复窗口的报告。
	ColumnDiagnostics(pos celler.ChunkPos) []Issue
}

type MemorySections map[dispatcher.SectionPos]*dispatcher.Section

// 交给浏览服务后，map 和 section 内容只读；修改内容时建立新的 View。

func (s MemorySections) Section(ctx context.Context, pos dispatcher.SectionPos) (*dispatcher.Section, error) {
	// 缺失位置直接作为空气，便于用少量 section 组装场景。
	return s[pos], ctx.Err()
}

func GroupBlockEntities(entities []java.BlockEntity) map[int32]map[dispatcher.CellPos]nbt.Compound {
	// Java 读取器与转换器使用相同的实体类型，统一换算负高度与格内坐标。
	// entities 属于同一根柱子，返回的 Compound 沿用输入，构建期间保持只读。
	grouped := map[int32]map[dispatcher.CellPos]nbt.Compound{}
	for _, entity := range entities {
		y := entity.Pos.Y >> 4
		if grouped[y] == nil {
			grouped[y] = map[dispatcher.CellPos]nbt.Compound{}
		}
		local := dispatcher.CellPos{
			uint8(entity.Pos.X & 15),
			uint8(entity.Pos.Y & 15),
			uint8(entity.Pos.Z & 15),
		}
		grouped[y][local] = entity.NBT
	}
	return grouped
}
