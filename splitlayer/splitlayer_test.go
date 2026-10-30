package splitlayer

import (
	"slices"
	"testing"

	"neomega-blocks/ir"
)

func TestSplitRulesAndHashes(t *testing.T) {
	splitter := New()
	for _, test := range []struct {
		name  string
		input ir.IRBlock
		want  []ir.IRBlock
	}{
		{"stone", `minecraft:stone []`, []ir.IRBlock{`minecraft:stone []`}},
		{"dry slab", `minecraft:oak_slab ["waterlogged"="false","type"="bottom"]`, []ir.IRBlock{`minecraft:oak_slab ["type"="bottom"]`}},
		{"wet slab", `minecraft:oak_slab ["waterlogged"="true","type"="bottom"]`, []ir.IRBlock{`minecraft:oak_slab ["type"="bottom"]`, `minecraft:water ["level"="0"]`}},
		{"kelp", `minecraft:kelp ["age"="0"]`, []ir.IRBlock{`minecraft:kelp ["age"="0"]`, `minecraft:water ["level"="0"]`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			want := make([]JavaBlockHashRSL, len(test.want))
			for i, block := range test.want {
				want[i] = block.Tidy().Hash()
			}
			if got := SplitBlock(test.input); !slices.Equal(got, want) {
				t.Fatalf("state rule = %v, want %v", got, want)
			}
			hash := test.input.Tidy().Hash()
			if got := splitter.SplitHash(hash); !slices.Equal(got, want) {
				t.Fatalf("hash expansion = %v, want %v", got, want)
			}
		})
	}
}

func TestSubchunkPaddingAndUnknownHash(t *testing.T) {
	splitter := New()
	air := ir.IRBlock("minecraft:air []").Tidy().Hash()
	stone := ir.IRBlock("minecraft:stone []").Tidy().Hash()
	kelp := ir.IRBlock(`minecraft:kelp ["age"="0"]`).Tidy().Hash()
	water := ir.IRBlock(`minecraft:water ["level"="0"]`).Tidy().Hash()
	unknown := ir.IRBlock("example:custom []").Tidy().Hash()
	blocks := make([]JavaBlockHash, 4096)
	for i := range blocks {
		blocks[i] = stone
	}
	if got := splitter.SplitSubchunk(blocks); len(got) != 1 {
		t.Fatalf("single layer expanded to %d layers", len(got))
	}
	blocks[123], blocks[456] = kelp, unknown
	original := slices.Clone(blocks)
	layers := splitter.SplitSubchunk(blocks)
	if len(layers) != 2 || len(layers[0]) != 4096 || len(layers[1]) != 4096 {
		t.Fatalf("unexpected layer dimensions: %v", layers)
	}
	if !slices.Equal(layers[0], blocks) || !slices.Equal(blocks, original) {
		t.Fatal("foreground or caller input changed")
	}
	for i, hash := range layers[1] {
		want := air
		if i == 123 {
			want = water
		}
		if hash != want {
			t.Fatalf("background cell %d = %d, want %d", i, hash, want)
		}
	}
	if got := splitter.SplitHash(unknown); !slices.Equal(got, []JavaBlockHashRSL{unknown}) {
		t.Fatalf("unknown hash changed: %v", got)
	}
}
