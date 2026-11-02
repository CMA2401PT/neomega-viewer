package javaworld

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"celler"
	"celler/java"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer/source"
)

type decodedColumn struct {
	// 请求内保留解码结果与按 section 分组的实体，六向访问不会重复解析区块。
	chunk    *java.Chunk
	entities map[int32]map[dispatcher.CellPos]nbt.Compound
	issues   []source.Issue
}

type sectionSource struct {
	source  *Source
	columns map[celler.ChunkPos]*decodedColumn
}

func (s *sectionSource) Section(ctx context.Context, pos dispatcher.SectionPos) (*dispatcher.Section, error) {
	// 原生坐标在适配器中换算，Pipeline 只接收完整 Java 状态和格内实体。
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cp := celler.ChunkPos{
		X: pos.X,
		Z: pos.Z,
	}
	column, found := s.columns[cp]
	if !found {
		raw, err := s.source.read(ctx, cp)
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
		column = &decodedColumn{chunk: chunk}
		entities, issues, err := chunk.BlockEntities()
		if err != nil {
			column.issues = append(column.issues, source.Issue{
				Count:  1,
				Reason: "invalid_block_entities",
				Detail: err.Error(),
			})
		}
		for _, issue := range issues {
			column.issues = append(column.issues, source.Issue{
				Count:  1,
				Reason: "invalid_block_entity",
				Detail: fmt.Sprintf("block_entities[%d]: %v", issue.Index, issue.Err),
			})
		}
		column.entities = source.GroupBlockEntities(entities)
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
	return &dispatcher.Section{
		Blocks:   section.Blocks,
		Entities: column.entities[pos.Y],
	}, nil
}

func (s *sectionSource) ColumnDiagnostics(pos celler.ChunkPos) []source.Issue {
	// 诊断随整列解码产生，包括请求高度以外的实体，但不混入邻居。
	if column := s.columns[pos]; column != nil {
		return column.issues
	}
	return nil
}
