package column_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"reflect"
	"testing"

	"celler"
	"neomega-blocks/ir"
	"neomega-viewer-core/viewer/blockdata"
	"neomega-viewer-core/viewer/cube"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer-core/viewer/merger"
	preset "neomega-viewer-core/viewer/resources"
	"neomega-viewer/column"
	"neomega-viewer/source"
)

type diagnosticSections struct {
	source.MemorySections
	issues  map[celler.ChunkPos][]source.Issue
	queried []celler.ChunkPos
}

func (s *diagnosticSections) ColumnDiagnostics(pos celler.ChunkPos) []source.Issue {
	s.queried = append(s.queried, pos)
	return s.issues[pos]
}

func TestMemoryColumnMeshAndSourceDiagnostics(t *testing.T) {
	resources, err := preset.Open(preset.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pipeline := dispatcher.NewJavaPipeline(cube.New(16), resources.Tables, nil, nil)
	center := celler.ChunkPos{
		X: -2,
		Z: 3,
	}
	neighbor := celler.ChunkPos{
		X: -1,
		Z: 3,
	}
	sections := &diagnosticSections{
		MemorySections: source.MemorySections{
			{
				X: -2,
				Y: 0,
				Z: 3,
			}: {Blocks: blockdata.Uniform(4096, ir.IRBlock("minecraft:stone []"))},
			{
				X: -1,
				Y: 0,
				Z: 3,
			}: {Blocks: blockdata.Uniform(4096, ir.IRBlock("minecraft:stone []"))},
		},
		issues: map[celler.ChunkPos][]source.Issue{
			center: {
				{
					Reason:     "unknown_block",
					Blockstate: "example:a []",
					Count:      7,
				},
				{
					Reason:     "unknown_block",
					Blockstate: "example:b []",
					Count:      2,
				},
				{
					Reason:        "unsupported_block_entity",
					BlockEntityID: "example:entity_a",
					Count:         3,
				},
				{
					Reason:        "unsupported_block_entity",
					BlockEntityID: "example:entity_b",
					Count:         4,
				},
				{
					Reason:   "invalid_block_entity",
					Position: &[3]int32{-31, 200, 49},
					Count:    1,
					Detail:   "outside window",
				},
				{
					Reason:   "invalid_block_entity",
					Position: &[3]int32{-31, 200, 49},
					Count:    1,
					Detail:   "outside window",
				},
			},
			neighbor: {{
				Reason: "neighbor_only",
				Count:  100,
			}},
		},
	}
	var first []column.Issue
	for _, y := range []int32{0, 4} {
		packet, err := column.BuildColumn(
			context.Background(),
			pipeline,
			sections,
			column.Key{
				X:    -2,
				Z:    3,
				MinY: y,
				MaxY: y,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if binary.LittleEndian.Uint32(packet[12:]) != 0 {
			t.Fatal("source counts changed missing-model count")
		}
		if y == 0 {
			if binary.LittleEndian.Uint32(packet[8:]) != 1 {
				t.Fatal("memory section produced no frame")
			}
			size := int(binary.LittleEndian.Uint32(packet[24:]))
			frame, err := merger.Unmarshal(bytes.NewReader(packet[32 : 32+size]))
			if err != nil {
				t.Fatal(err)
			}
			if len(frame.Mesh.Quads) != 5 {
				t.Fatalf("neighbor culling: %d quads, want 5", len(frame.Mesh.Quads))
			}
		}
		size := int(binary.LittleEndian.Uint32(packet[16:]))
		var issues []column.Issue
		if err = json.Unmarshal(packet[len(packet)-size:], &issues); err != nil {
			t.Fatal(err)
		}
		if len(issues) != 6 {
			t.Fatalf("source entries merged or neighbor leaked: %+v", issues)
		}
		for i, issue := range issues {
			want := sections.issues[center][i]
			if issue.Origin != "source" || issue.Reason != want.Reason || issue.Count != want.Count || issue.Blockstate != want.Blockstate || issue.BlockEntityID != want.BlockEntityID || issue.Detail != want.Detail {
				t.Fatalf("source issue changed: %+v", issue)
			}
			if i < 4 && (issue.Scope != "column" || issue.Position != [3]int32{-32, 0, 48}) {
				t.Fatal(issue)
			}
			if i >= 4 && (issue.Scope != "block" || issue.Position != *want.Position) {
				t.Fatal(issue)
			}
		}
		if y == 0 {
			first = issues
		} else if !reflect.DeepEqual(first, issues) {
			t.Fatal("column diagnostics depend on height window")
		}
	}
	if !reflect.DeepEqual(sections.queried, []celler.ChunkPos{center, center}) {
		t.Fatal(sections.queried)
	}
}
