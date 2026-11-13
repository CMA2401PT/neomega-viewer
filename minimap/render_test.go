package minimap

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"testing"

	"celler"
	"neomega-viewer-core/viewer/blockdata"
	"neomega-viewer-core/viewer/brickmap"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer/source"
)

func TestRegionPreservesUndergroundAndNegativeCoordinates(t *testing.T) {
	materials, err := brickmap.OpenMaterialSource()
	if err != nil {
		t.Fatal(err)
	}
	blocks := blockdata.Uniform(4096, "minecraft:stone []")
	blocks.Set(15<<8|15<<4|7, "minecraft:cave_air []")
	blocks.Set(15<<8|15<<4|8, `minecraft:water ["level"="0"]`)
	sections := source.MemorySections{
		{X: -1, Y: -4, Z: -1}: &dispatcher.Section{Blocks: blocks},
		{X: -1, Y: 19, Z: -1}: &dispatcher.Section{Blocks: blockdata.Uniform(4096, "minecraft:gold_block []")},
	}
	terrain := Terrain{Materials: materials, MinSection: -4, MaxSection: 19}
	volume, err := terrain.Region(context.Background(), func() dispatcher.SectionSource { return sections }, Position{X: -1, Z: -1})
	if err != nil {
		t.Fatal(err)
	}
	if volume.Origin != [3]int{-128, -64, -128} || volume.Size != [3]int{128, 512, 128} {
		t.Fatal(volume.Origin, volume.Size)
	}
	if volume.At([3]int{-1, -57, -1}) != 0 || volume.At([3]int{-1, -56, -1}) == 0 || volume.At([3]int{-1, -58, -1}) == 0 {
		t.Fatal("地下洞穴或水丢失")
	}
	if volume.At([3]int{-1, 319, -1}) == 0 || volume.At([3]int{-1, 320, -1}) != 0 {
		t.Fatal("真实高度与补齐高度混淆")
	}
	water := volume.Palette[volume.At([3]int{-1, -56, -1})]
	if water.Group == 0 || water.Faces[3][3] == 255 {
		t.Fatal("透明材质参数丢失", water)
	}
	if volume.At([3]int{-128, 0, -128}) != 0 {
		t.Fatal("不存在的 section 不为空气")
	}
}

func TestRegionCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (Terrain{MinSection: 0, MaxSection: 0}).Region(ctx, func() dispatcher.SectionSource { return source.MemorySections{} }, Position{})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestOverviewNegativeOrigin(t *testing.T) {
	m := New(Options{Chunks: func(context.Context) ([]celler.ChunkPos, error) { return []celler.ChunkPos{{X: -1, Z: -1}}, nil }})
	defer m.Retire()
	data, err := m.Tile(context.Background(), Position{X: -1, Z: -1})
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 128 || img.Bounds().Dy() != 128 {
		t.Fatal(img.Bounds())
	}
	_, _, _, alpha := img.At(127, 127).RGBA()
	if alpha == 0 {
		t.Fatal("负坐标区块未落到右下角")
	}
	_, _, _, alpha = img.At(0, 0).RGBA()
	if alpha != 0 {
		t.Fatal("缺失区块不透明")
	}
}
