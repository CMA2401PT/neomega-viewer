package minimap

import (
	"bytes"
	"context"
	"image/png"
	"reflect"
	"testing"

	"celler"
	"neomega-viewer-core/viewer/blockdata"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer-core/viewer/topview"
	"neomega-viewer/source"
)

type recordingSource struct {
	sections source.MemorySections
	ys       []int32
}

func (s *recordingSource) Section(ctx context.Context, pos dispatcher.SectionPos) (*dispatcher.Section, error) {
	// 记录实际读取高度，使提前停止与跨 section 的顶棚规则可以直接观察。
	s.ys = append(s.ys, pos.Y)
	return s.sections.Section(ctx, pos)
}

func TestColumnTopSelectionAndEarlyStop(t *testing.T) {
	colors, err := topview.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	blocks := blockdata.Uniform(4096, "minecraft:air []")
	for z := range 16 {
		for x := range 16 {
			blocks.Set(x<<8|z<<4|3, "minecraft:stone []")
		}
	}
	blocks.Set(2<<8|4<<4|9, "minecraft:gold_block []")
	blocks.Set(2<<8|4<<4|15, "minecraft:light []")
	reader := &recordingSource{sections: source.MemorySections{
		{Y: 1}: &dispatcher.Section{Blocks: blocks},
		{Y: 0}: &dispatcher.Section{Blocks: blockdata.Uniform(4096, "minecraft:dirt []")},
	}}
	terrain := Terrain{Colors: colors, MinSection: -4, MaxSection: 2}
	column, err := terrain.Column(context.Background(), reader, celler.ChunkPos{})
	if err != nil {
		t.Fatal(err)
	}
	if column.Height[4*16+2] != 25 || column.Height[0] != 19 {
		t.Fatal(column.Height)
	}
	if !reflect.DeepEqual(reader.ys, []int32{2, 1}) {
		t.Fatal("没有在全部像素取得地表后停止", reader.ys)
	}
	rgb, _ := colors.Color("minecraft:gold_block []")
	pixel := column.Pixel[4*16+2]
	if pixel.R != uint8(rgb>>16) || pixel.G != uint8(rgb>>8) || pixel.B != uint8(rgb) || pixel.A != 255 {
		t.Fatal(pixel, rgb)
	}
}

func TestNetherRoofAcrossSectionsAndMissingAir(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{true: "missing_air", false: "air_gap"}[missing], func(t *testing.T) {
			sections := source.MemorySections{
				{Y: 7}: &dispatcher.Section{Blocks: blockdata.Uniform(4096, "minecraft:bedrock []")},
				{Y: 6}: &dispatcher.Section{Blocks: blockdata.Uniform(4096, "minecraft:netherrack []")},
				{Y: 3}: &dispatcher.Section{Blocks: blockdata.Uniform(4096, "minecraft:stone []")},
			}
			if !missing {
				blocks := blockdata.Uniform(4096, "minecraft:netherrack []")
				for z := range 16 {
					for x := range 16 {
						blocks.Set(x<<8|z<<4|7, "minecraft:cave_air []")
					}
				}
				sections[dispatcher.SectionPos{Y: 5}] = &dispatcher.Section{Blocks: blocks}
			}
			reader := &recordingSource{sections: sections}
			column, err := (Terrain{MinSection: 0, MaxSection: 7, Nether: true}).Column(context.Background(), reader, celler.ChunkPos{})
			if err != nil {
				t.Fatal(err)
			}
			want := int32(86)
			if missing {
				want = 63
			}
			for _, y := range column.Height {
				if y != want {
					t.Fatalf("地表 %d，期望 %d", y, want)
				}
			}
		})
	}
}

func TestTileDimensionsAndNegativeOrigin(t *testing.T) {
	sections := source.MemorySections{
		{X: -1, Y: 0, Z: -1}: &dispatcher.Section{Blocks: blockdata.Uniform(4096, "minecraft:stone []")},
	}
	terrain := Terrain{MinSection: 0, MaxSection: 0}
	data, err := terrain.Tile(context.Background(), func() dispatcher.SectionSource { return sections }, Position{X: -1, Z: -1})
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dx() != 128 || img.Bounds().Dy() != 128 {
		t.Fatal(img, err)
	}
	_, _, _, alpha := img.At(127, 127).RGBA()
	if alpha == 0 {
		t.Fatal("负坐标柱子没有画到图块右下角")
	}
	_, _, _, alpha = img.At(0, 0).RGBA()
	if alpha != 0 {
		t.Fatal("不存在的柱子应透明")
	}
}
