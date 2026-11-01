package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"

	"celler"
	"celler/java"
	core "neomega-blocks/geometry"
	jmodel "neomega-blocks/geometry/java_block_model"
	"neomega-blocks/ir"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer/dispatcher"
)

const maxColumnBytes = 24 << 20

type chunkKey struct{ X, Z, MinY, MaxY int32 }

// RenderIssue retains the source blockstate before layer splitting. An empty
// blockstate identifies a column-level issue; Position is then the column origin.
type RenderIssue struct {
	Position   [3]int32 `json:"position"`
	Blockstate string   `json:"blockstate"`
	Reason     string   `json:"reason"`
	Detail     string   `json:"detail,omitempty"`
}

// Engine is immutable after creation. Request workers own all temporary arrays.
type Engine struct {
	pipeline *dispatcher.Pipeline
}

func NewEngine(tables *core.Tables[uint32], atlas *jmodel.Atlas) (*Engine, error) {
	// 占位只持有石头形状的独立副本，不改变共享几何表的 ID 空间。
	stone, err := tables.ModelIDs(ir.IRBlock("minecraft:stone []").Hash())
	if err != nil || len(stone) == 0 {
		return nil, fmt.Errorf("geometry table requires stone for missing-state marker")
	}
	shape := tables.Shapes[stone[0]]
	shape.Quads = append([]core.Quad(nil), shape.Quads...)
	shape.CullGroup = 0
	for i := range shape.Quads {
		shape.Quads[i].Surface.Unlit = true
		shape.Quads[i].Color = [3]float32{1, 0, 1}
	}
	return &Engine{pipeline: dispatcher.NewJavaPipeline(tables, atlas, &shape)}, nil
}

type decodedColumn struct {
	// 请求内保留解码结果与按 section 分组的实体，六向访问不会重复解析区块。
	chunk    *java.Chunk
	entities map[int32]map[dispatcher.CellPos]nbt.Compound
}

type sectionSource struct {
	source  *Source
	columns map[celler.ChunkPos]*decodedColumn
	issues  []RenderIssue
}

func (s *sectionSource) Section(ctx context.Context, pos dispatcher.SectionPos) (*dispatcher.Section, error) {
	// 原生坐标在适配器中换算，Pipeline 只接收完整 Java 状态和格内实体。
	cp := celler.ChunkPos{X: pos.X, Z: pos.Z}
	column, found := s.columns[cp]
	if !found {
		raw, err := s.source.Read(ctx, cp)
		if errors.Is(err, fs.ErrNotExist) {
			s.columns[cp] = nil
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		data, err := raw.Decode()
		if err != nil {
			return nil, fmt.Errorf("chunk %v: %w", cp, err)
		}
		chunk, err := java.DecodeChunk(data)
		if err != nil {
			return nil, err
		}
		actual, err := chunk.Position()
		if err != nil {
			return nil, err
		}
		if actual != cp {
			return nil, fmt.Errorf("chunk position %v does not match %v", actual, cp)
		}
		column = &decodedColumn{
			chunk:    chunk,
			entities: map[int32]map[dispatcher.CellPos]nbt.Compound{},
		}
		entities, issues, err := chunk.BlockEntities()
		if err != nil {
			s.issues = append(s.issues, RenderIssue{
				Position: [3]int32{cp.X * 16, 0, cp.Z * 16},
				Reason:   "invalid_block_entities",
				Detail:   err.Error(),
			})
		}
		for _, issue := range issues {
			s.issues = append(s.issues, RenderIssue{
				Position: [3]int32{cp.X * 16, 0, cp.Z * 16},
				Reason:   "invalid_block_entity",
				Detail:   fmt.Sprintf("block_entities[%d]: %v", issue.Index, issue.Err),
			})
		}
		for _, entity := range entities {
			y := entity.Pos.Y >> 4
			if column.entities[y] == nil {
				column.entities[y] = map[dispatcher.CellPos]nbt.Compound{}
			}
			local := dispatcher.CellPos{
				uint8(entity.Pos.X & 15),
				uint8(entity.Pos.Y & 15),
				uint8(entity.Pos.Z & 15),
			}
			column.entities[y][local] = entity.NBT
		}
		s.columns[cp] = column
	}
	if column == nil {
		return nil, nil
	}
	section, err := column.chunk.Section(pos.Y)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("chunk %v section %d: %w", cp, pos.Y, err)
	}
	return &dispatcher.Section{Blocks: section.Blocks, Entities: column.entities[pos.Y]}, nil
}

// BuildColumn returns a bounded NVWC response containing independently positioned
// section frames. Only the requested vertical window and touching neighbours decode.
func (e *Engine) BuildColumn(ctx context.Context, source *Source, key chunkKey) ([]byte, error) {
	adapter := &sectionSource{
		source:  source,
		columns: map[celler.ChunkPos]*decodedColumn{},
	}
	var positions []dispatcher.SectionPos
	for y := key.MinY; y <= key.MaxY; y++ {
		positions = append(positions, dispatcher.SectionPos{X: key.X, Y: y, Z: key.Z})
	}
	results, err := e.pipeline.BuildSections(ctx, adapter, positions, dispatcher.Options{})
	if err != nil {
		return nil, err
	}
	var issues []RenderIssue
	reported := map[string]bool{}
	report := func(issue RenderIssue) {
		key := issue.Blockstate + ":" + issue.Reason
		if issue.Blockstate == "" {
			key = fmt.Sprintf("%s:%v", issue.Reason, issue.Position)
		}
		if !reported[key] {
			reported[key] = true
			issues = append(issues, issue)
		}
	}
	for _, issue := range adapter.issues {
		report(issue)
	}
	out := bytes.NewBuffer(emptyPacket())
	count, unknown := uint32(0), uint32(0)
	for _, positioned := range results {
		result := positioned.Result
		missing := uint32(0)
		for _, diagnostic := range result.Diagnostics {
			issue := RenderIssue{
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
