package demo_test

import (
	"testing"

	"celler/layer_storage"
	"neomega-blocks/ir"
	java "neomega-blocks/presets/java/v1"
	"neomega-converter/fixer"
	"neomega-viewer/demo/scenes"
	"neomega-viewer/dispatcher"
)

func TestSpecialNEMCSourceData(t *testing.T) {
	// 这些外观只能从源 NBT 和第二层水恢复，必须在 fixer 之前就正确。
	scene, err := demo.BuildWithNBT("special_nemc")
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		position [3]uint8
		want     ir.IRBlock
	}{
		{[3]uint8{2, 1, 13}, `minecraft:blue_bed ["facing"="east","occupied"="false","part"="foot"]`},
		{[3]uint8{3, 1, 13}, `minecraft:blue_bed ["facing"="east","occupied"="false","part"="head"]`},
		{[3]uint8{5, 1, 13}, `minecraft:yellow_bed ["facing"="south","occupied"="false","part"="foot"]`},
		{[3]uint8{5, 1, 14}, `minecraft:yellow_bed ["facing"="south","occupied"="false","part"="head"]`},
		{[3]uint8{9, 1, 13}, `minecraft:lime_bed ["facing"="west","occupied"="false","part"="foot"]`},
		{[3]uint8{8, 1, 13}, `minecraft:lime_bed ["facing"="west","occupied"="false","part"="head"]`},
		{[3]uint8{3, 1, 11}, `minecraft:red_bed ["facing"="north","occupied"="false","part"="foot"]`},
		{[3]uint8{3, 1, 10}, `minecraft:red_bed ["facing"="north","occupied"="false","part"="head"]`},
		{[3]uint8{11, 1, 11}, "minecraft:potted_cactus []"},
		{[3]uint8{7, 1, 7}, `minecraft:oak_slab ["type"="bottom","waterlogged"="true"]`},
	} {
		position := layer_storage.Position{X: check.position[0], Y: check.position[1], Z: check.position[2]}
		if got := scene.Blocks.Get(position); got != check.want.Tidy().Hash() {
			block, _ := java.HashToBlock(got)
			t.Errorf("%v: got %d (%s), want %s", check.position, got, block, check.want)
		}
	}
}

func TestSpecialNEMCMatchesJavaAfterFix(t *testing.T) {
	// 逐格对照完整布局，确保其余差异都能由 fixer 根据邻居补回。
	nemcLayer, err := demo.Build("special_nemc")
	if err != nil {
		t.Fatal(err)
	}
	javaLayer, err := demo.Build("special_java")
	if err != nil {
		t.Fatal(err)
	}
	var source, reference layer_storage.Plain[uint32]
	source.CopyFrom(nemcLayer)
	reference.CopyFrom(javaLayer)
	stateFixer, err := fixer.New()
	if err != nil {
		t.Fatal(err)
	}
	states, err := dispatcher.BlocksFromHashes(source.Data)
	if err != nil {
		t.Fatal(err)
	}
	fixed := stateFixer.FixSection(states, [6]*layer_storage.Palette[ir.IRBlock]{})
	var repaired layer_storage.Plain[uint32]
	repaired.CopyFrom(layer_storage.ConvertPalette(fixed, java.BlockToHash))
	for _, stage := range []struct {
		name   string
		blocks []uint32
	}{
		{"before fix", source.Data},
		{"after fix", repaired.Data},
	} {
		differences := 0
		for index, got := range stage.blocks {
			want := reference.Data[index]
			if got == want {
				continue
			}
			differences++
			block, _ := java.HashToBlock(got)
			expected, _ := java.HashToBlock(want)
			t.Logf("%s (%d,%d,%d): NEMC %d (%s); Java %d (%s)",
				stage.name,
				index>>8,
				index&15,
				(index>>4)&15,
				got,
				block,
				want,
				expected,
			)
		}
		t.Logf("%s: %d different cells", stage.name, differences)
		if stage.name == "after fix" && differences != 0 {
			t.Errorf("fixed special_nemc differs from special_java at %d cells", differences)
		}
	}
}
