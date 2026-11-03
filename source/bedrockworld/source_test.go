package bedrockworld_test

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"celler"
	native "celler/bedrock"
	"celler/java"
	"celler/layer_storage"
	"neomega-blocks/ir"
	"neomega-converter-core"
	convert "neomega-converter-core/bedrock"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer-core/viewer/blockactor"
	"neomega-viewer-core/viewer/cube"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer-core/viewer/effector"
	preset "neomega-viewer-core/viewer/resources"
	"neomega-viewer/column"
	"neomega-viewer/source"
	"neomega-viewer/source/bedrockworld"
)

type chunks struct {
	values map[celler.ChunkPos]*converter.ConvertedChunk
	calls  map[celler.ChunkPos]int
	err    error
}

func (s *chunks) Chunk(ctx context.Context, pos celler.ChunkPos) (*converter.ConvertedChunk, error) {
	s.calls[pos]++
	return s.values[pos], s.err
}

func TestColumnDiagnosticsKeepConversionCounts(t *testing.T) {
	// 未知状态真实经过转换变为空气，再走同一个列构建入口，验证不会变成紫色占位。
	layer := native.NewLayer(native.Block{State: "minecraft:air []"})
	for x := range 7 {
		layer.Blocks.Set(layer_storage.At(x, 0, 0), native.Block{State: "example:unknown []"})
	}
	layer.Blocks.Set(layer_storage.At(8, 0, 0), native.Block{State: "minecraft:stone []"})
	center := convert.ConvertSection(
		celler.ChunkPos{},
		0,
		&native.Section{
			Y:      0,
			Layers: []*native.Layer{layer},
		},
		nil,
	)
	center.Diagnostics = append(center.Diagnostics,
		converter.Diagnostic{
			Kind:          converter.UnsupportedBlockEntity,
			BlockEntityID: "example:entity",
			Count:         3,
		},
		converter.Diagnostic{
			Kind: converter.NBTError,
			Err:  errors.New("broken entity NBT"),
		},
		converter.Diagnostic{
			Kind: converter.RuleError,
			Pos: &celler.BlockPos{
				X: 1,
				Y: 200,
				Z: 2,
			},
			Err: errors.New("outside requested height"),
		},
	)
	src := bedrockworld.New(&chunks{
		values: map[celler.ChunkPos]*converter.ConvertedChunk{
			{}: center,
			{X: 1}: {
				Diagnostics: []converter.Diagnostic{{
					Kind:  converter.UnknownBlock,
					State: "example:neighbor []",
					Count: 99,
				}},
			},
		},
		calls: make(map[celler.ChunkPos]int),
	})
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pipeline := dispatcher.NewPipeline(cube.New(16), resources.Tables, effector.New(nil), blockactor.New(), nil)
	var first []column.Issue
	for _, y := range []int32{0, 8} {
		packet, err := column.BuildColumn(
			context.Background(),
			pipeline,
			src.Reader(),
			column.Key{
				MinY: y,
				MaxY: y,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if binary.LittleEndian.Uint32(packet[12:]) != 0 {
			t.Fatal("conversion unknowns increased missing-model count")
		}
		if y == 0 && binary.LittleEndian.Uint32(packet[8:]) != 1 {
			t.Fatal("converted stone produced no mesh frame")
		}
		size := int(binary.LittleEndian.Uint32(packet[16:]))
		var issues []column.Issue
		if err := json.Unmarshal(packet[len(packet)-size:], &issues); err != nil {
			t.Fatal(err)
		}
		if len(issues) != 4 {
			t.Fatalf("diagnostics lost or neighbor leaked: %+v", issues)
		}
		if issues[0].Scope != "column" || issues[0].Blockstate != "example:unknown []" || issues[0].Count != 7 {
			t.Fatal(issues[0])
		}
		if issues[1].BlockEntityID != "example:entity" || issues[1].Count != 3 {
			t.Fatal(issues[1])
		}
		if issues[2].Count != 1 || issues[2].Detail != "broken entity NBT" {
			t.Fatal(issues[2])
		}
		if issues[3].Position != [3]int32{1, 200, 2} || issues[3].Scope != "block" {
			t.Fatal(issues[3])
		}
		for _, issue := range issues {
			if issue.Origin != "source" {
				t.Fatal(issue)
			}
		}
		if y == 0 {
			first = issues
		} else if !reflect.DeepEqual(first, issues) {
			t.Fatal("source diagnostics depend on height window")
		}
	}
}

func TestReadersBorrowColumnsAndGroupEntities(t *testing.T) {
	palette := layer_storage.MakePaletteStorage(ir.IRBlock("minecraft:stone []"))
	pos := celler.ChunkPos{X: -1}
	src := &chunks{
		values: map[celler.ChunkPos]*converter.ConvertedChunk{
			pos: {
				Sections: map[int32]*converter.ConvertedSection{
					-1: {Blocks: palette},
				},
				BlockEntities: []java.BlockEntity{{
					Pos: celler.BlockPos{
						X: -1,
						Y: -1,
						Z: 2,
					},
					NBT: nbt.Compound{"id": "example:entity"},
				}},
			},
		},
		calls: make(map[celler.ChunkPos]int),
	}
	s := bedrockworld.New(src)
	for range 2 {
		reader := s.Reader()
		for range 2 {
			section, err := reader.Section(context.Background(), dispatcher.SectionPos{
				X: -1,
				Y: -1,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(section.Blocks.Indices) != 4096 || section.Blocks.Get(0) != palette.Get(layer_storage.At(0, 0, 0)) || section.Entities[dispatcher.CellPos{15, 15, 2}]["id"] != "example:entity" {
				t.Fatal("block data or entity misplaced")
			}
			if section, err := reader.Section(context.Background(), dispatcher.SectionPos{}); err != nil || section != nil {
				t.Fatal(section, err)
			}
		}
		if len(reader.(source.ColumnDiagnostics).ColumnDiagnostics(pos)) != 0 {
			t.Fatal("unexpected diagnostics")
		}
	}
	if src.calls[pos] != 2 || src.calls[celler.ChunkPos{}] != 2 {
		t.Fatal("column or absence not retained within reader", src.calls)
	}
	src.err = errors.New("read failed")
	if _, err := s.Reader().Section(context.Background(), dispatcher.SectionPos{}); !errors.Is(err, src.err) {
		t.Fatal(err)
	}
}
