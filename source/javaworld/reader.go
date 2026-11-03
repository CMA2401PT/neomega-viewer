package javaworld

import (
	"celler"
	"celler/java"
	"context"
	"errors"
	"fmt"
	"io/fs"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer-core/viewer/entity_render"
	"neomega-viewer/source"
)

type decodedColumn struct {
	// 请求内共享解码结果；地形缺失时，实体记录仍可独立产生 section。
	chunk         *java.Chunk
	blockEntities map[int32]map[dispatcher.CellPos]nbt.Compound
	entities      map[dispatcher.SectionPos]*entity_render.Input
	issues        []source.Issue
}

type sectionSource struct {
	source  *Source
	columns map[celler.ChunkPos]*decodedColumn
}

func (s *sectionSource) Section(ctx context.Context, pos dispatcher.SectionPos) (*dispatcher.Section, error) {
	// 来源边界保留原始实体 NBT，并将世界浮点位置换成区域局部位置。
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cp := celler.ChunkPos{X: pos.X, Z: pos.Z}
	column, found := s.columns[cp]
	if !found {
		records, err := s.source.read(ctx, cp)
		if err != nil {
			return nil, err
		}
		column = &decodedColumn{}
		if records[0] != nil {
			data, err := records[0].Decode()
			if err != nil {
				return nil, fmt.Errorf("terrain %v: %w", cp, err)
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
			column.chunk = chunk
			entities, issues, err := chunk.BlockEntities()
			if err != nil {
				column.issues = append(column.issues, source.Issue{Count: 1, Reason: "invalid_block_entities", Detail: err.Error()})
			}
			for _, issue := range issues {
				column.issues = append(column.issues, source.Issue{Count: 1, Reason: "invalid_block_entity", Detail: fmt.Sprintf("block_entities[%d]: %v", issue.Index, issue.Err)})
			}
			column.blockEntities = source.GroupBlockEntities(entities)
		}
		if records[1] != nil {
			data, err := records[1].Decode()
			if err != nil {
				return nil, fmt.Errorf("entities %v: %w", cp, err)
			}
			document, err := nbt.DecodeDocument(data, nbt.EncodingBigEndian)
			if err != nil {
				return nil, fmt.Errorf("entities %v: %w", cp, err)
			}
			entities, ok := nbt.Compounds(document.Value["Entities"])
			if !ok {
				column.issues = append(column.issues, source.Issue{Count: 1, Reason: "invalid_entities", Detail: "Entities must be a compound list"})
			} else {
				var issues []source.Issue
				column.entities, issues = source.GroupEntities(entities)
				column.issues = append(column.issues, issues...)
			}
		}
		s.columns[cp] = column
	}
	result := &dispatcher.Section{BlockEntities: column.blockEntities[pos.Y], Entities: column.entities[pos]}
	if column.chunk != nil {
		section, err := column.chunk.Section(pos.Y)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("chunk %v section %d: %w", cp, pos.Y, err)
		}
		if section != nil {
			result.Blocks = source.BlocksFromPalette(section.Blocks)
		}
	}
	if result.Blocks == nil && result.Entities == nil {
		return nil, nil
	}
	return result, nil
}

func (s *sectionSource) ColumnDiagnostics(pos celler.ChunkPos) []source.Issue {
	// 整列诊断包括请求高度以外的实体，但不混入邻居。
	if column := s.columns[pos]; column != nil {
		return column.issues
	}
	return nil
}
