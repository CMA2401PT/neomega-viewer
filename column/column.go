package column

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"

	"celler"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer/source"
)

const maxColumnBytes = 24 << 20

type Key struct {
	// 使用区块 X/Z 和含首尾的 section Y 范围，供列构建和响应缓存共同定位。
	X, Z, MinY, MaxY int32
}

type Issue struct {
	// Scope 区分方块、普通实体与整列；整列 Position 为柱子原点。
	// Origin 区分来源与渲染两种去重规则，不依赖 Reason 名称或是否带有状态。
	Scope         string   `json:"scope"`
	Origin        string   `json:"origin"`
	Position      [3]int32 `json:"position"`
	Blockstate    string   `json:"blockstate,omitempty"`
	Reason        string   `json:"reason"`
	Count         int      `json:"count"`
	BlockEntityID string   `json:"blockEntityId,omitempty"`
	EntityID      string   `json:"entityId,omitempty"`
	Detail        string   `json:"detail,omitempty"`
}

func BuildColumn(
	ctx context.Context,
	pipeline *dispatcher.Pipeline,
	sections dispatcher.SectionSource,
	key Key,
) ([]byte, error) {
	// 每次构建只处理请求窗口，来源自行保留本次调用的解码结果。
	// 来源诊断覆盖请求柱子的全部高度；邻居只参与剔面。返回字节供 NVWC 前端解码。
	var positions []dispatcher.SectionPos
	for y := key.MinY; y <= key.MaxY; y++ {
		positions = append(positions, dispatcher.SectionPos{
			X: key.X,
			Y: y,
			Z: key.Z,
		})
	}
	results, err := pipeline.BuildSections(
		ctx,
		sections,
		positions,
		dispatcher.Options{},
	)
	if err != nil {
		return nil, err
	}
	var issues []Issue
	reported := map[[2]string]bool{}
	report := func(issue Issue) {
		key := [2]string{issue.Blockstate, issue.Reason}
		if !reported[key] {
			reported[key] = true
			issues = append(issues, issue)
		}
	}
	if diagnostics, ok := sections.(source.ColumnDiagnostics); ok {
		for _, issue := range diagnostics.ColumnDiagnostics(celler.ChunkPos{
			X: key.X,
			Z: key.Z,
		}) {
			encoded := Issue{
				Scope:         "column",
				Origin:        "source",
				Position:      [3]int32{key.X * 16, 0, key.Z * 16},
				Blockstate:    issue.Blockstate,
				Reason:        issue.Reason,
				Count:         issue.Count,
				BlockEntityID: issue.BlockEntityID,
				Detail:        issue.Detail,
			}
			if issue.Position != nil {
				encoded.Scope = "block"
				encoded.Position = *issue.Position
			}
			issues = append(issues, encoded)
		}
	}
	out := bytes.NewBuffer(emptyPacket())
	count, unknown := uint32(0), uint32(0)
	for _, positioned := range results {
		result := positioned.Result
		missing := uint32(0)
		for _, diagnostic := range result.Diagnostics {
			issue := Issue{
				Scope:  "block",
				Origin: "render",
				Count:  1,
				Position: [3]int32{
					positioned.Pos.X*16 + int32(diagnostic.Pos[0]),
					positioned.Pos.Y*16 + int32(diagnostic.Pos[1]),
					positioned.Pos.Z*16 + int32(diagnostic.Pos[2]),
				},
				Blockstate: string(diagnostic.State),
				Reason:     string(diagnostic.Kind),
			}
			if diagnostic.Err != nil {
				issue.Detail = diagnostic.Err.Error()
			}
			if diagnostic.Kind == dispatcher.MissingModel {
				missing++
			}
			report(issue)
		}
		for _, diagnostic := range result.EntityDiagnostics {
			issues = append(issues, Issue{Scope: "entity", Origin: "render", Count: 1,
				Position: [3]int32{positioned.Pos.X*16 + int32(math.Floor(diagnostic.Pos[0])), positioned.Pos.Y*16 + int32(math.Floor(diagnostic.Pos[1])), positioned.Pos.Z*16 + int32(math.Floor(diagnostic.Pos[2]))},
				EntityID: diagnostic.ID, Reason: "entity_error", Detail: diagnostic.Err.Error(),
			})
		}
		unknown += missing
		if len(result.Frame.Mesh.Quads) == 0 && len(result.Frame.Actors) == 0 && len(result.Frame.Effects) == 0 {
			continue
		}
		var frame bytes.Buffer
		if err := result.Frame.Marshal(&frame); err != nil {
			return nil, err
		}
		if out.Len()+frame.Len()+12 > maxColumnBytes {
			return nil, fmt.Errorf("column mesh exceeds 24 MiB; reduce view distance")
		}
		binary.Write(out, binary.LittleEndian, positioned.Pos.Y)
		binary.Write(out, binary.LittleEndian, uint32(frame.Len()))
		binary.Write(out, binary.LittleEndian, missing)
		out.Write(frame.Bytes())
		count++
	}
	var diagnostics []byte
	if len(issues) > 0 {
		var err error
		diagnostics, err = json.Marshal(issues)
		if err != nil {
			return nil, err
		}
	}
	if out.Len()+len(diagnostics) > maxColumnBytes {
		return nil, fmt.Errorf("column diagnostics exceed 24 MiB; reduce view distance")
	}
	out.Write(diagnostics)
	data := out.Bytes()
	binary.LittleEndian.PutUint32(data[16:], uint32(len(diagnostics)))
	binary.LittleEndian.PutUint32(data[8:], count)
	binary.LittleEndian.PutUint32(data[12:], unknown)
	return data, nil
}
func emptyPacket() []byte {
	data := make([]byte, 20)
	copy(data, "NVWC")
	binary.LittleEndian.PutUint32(data[4:], 3)
	return data
}
