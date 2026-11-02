package bedrockworld

import (
	"context"

	"celler"
	"neomega-converter"
	"neomega-converter/adapt"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer/dispatcher"
	"neomega-viewer/source"
)

type Source struct {
	// 消费共享只读的转换柱子；是否修复以及底层句柄的生命周期由组装方决定。
	chunks adapt.ChunkSource
}

func New(chunks adapt.ChunkSource) *Source {
	// 转换与修复实现相同的取柱接口，渲染适配不需要区分这两种来源。
	return &Source{chunks: chunks}
}

func (s *Source) Reader() dispatcher.SectionSource {
	// 每次构建独占一个 reader，保留柱子引用和实体分组，缓存淘汰不影响本次构建。
	// 返回值同时实现 source.ColumnDiagnostics；单个 reader 串行使用。
	return &reader{
		chunks:  s.chunks,
		columns: make(map[celler.ChunkPos]*column),
	}
}

type column struct {
	chunk    *converter.ConvertedChunk
	entities map[int32]map[dispatcher.CellPos]nbt.Compound
	issues   []source.Issue
}

type reader struct {
	chunks  adapt.ChunkSource
	columns map[celler.ChunkPos]*column
}

func (r *reader) Section(ctx context.Context, pos dispatcher.SectionPos) (*dispatcher.Section, error) {
	// 调色板和实体 NBT 直接借用；普通实体没有对应的 dispatcher 输入。
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cp := celler.ChunkPos{
		X: pos.X,
		Z: pos.Z,
	}
	c, found := r.columns[cp]
	if !found {
		chunk, err := r.chunks.Chunk(ctx, cp)
		if err != nil {
			return nil, err
		}
		if chunk != nil {
			c = &column{
				chunk:    chunk,
				entities: source.GroupBlockEntities(chunk.BlockEntities),
				issues:   issuesFrom(chunk.Diagnostics),
			}
		}
		r.columns[cp] = c
	}
	if c == nil || c.chunk.Sections[pos.Y] == nil {
		return nil, nil
	}
	return &dispatcher.Section{
		Blocks:   c.chunk.Sections[pos.Y].Blocks,
		Entities: c.entities[pos.Y],
	}, nil
}

func (r *reader) ColumnDiagnostics(pos celler.ChunkPos) []source.Issue {
	// 查询整根柱子的诊断，既不按高度裁剪，也不混入修复或剔面所需邻居的报告。
	if c := r.columns[pos]; c != nil {
		return c.issues
	}
	return nil
}

func issuesFrom(diagnostics []converter.Diagnostic) []source.Issue {
	// converter 的单项 NBT/规则错误没有累计 Count，在浏览诊断中按一项表达。
	issues := make([]source.Issue, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		issue := source.Issue{
			Reason:        string(diagnostic.Kind),
			Blockstate:    string(diagnostic.State),
			Count:         max(1, int(diagnostic.Count)),
			BlockEntityID: diagnostic.BlockEntityID,
		}
		if diagnostic.Pos != nil {
			issue.Position = &[3]int32{
				diagnostic.Pos.X,
				diagnostic.Pos.Y,
				diagnostic.Pos.Z,
			}
		}
		if diagnostic.Err != nil {
			issue.Detail = diagnostic.Err.Error()
		}
		issues = append(issues, issue)
	}
	return issues
}
