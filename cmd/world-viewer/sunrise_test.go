package main

import (
	"context"
	"testing"

	"neomega-viewer-core/viewer/dispatcher"
)

func TestEmptySourceShowsSunrise(t *testing.T) {
	// 未打开存档时仍不算已载入，但来源提供海面与环，页面拿到取景信息。
	source, err := OpenSource(context.Background(), SourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	backdrop := source.Info.Backdrop
	if source.Info.Loaded || backdrop == nil || backdrop.Lanterns == 0 || backdrop.Glass == 0 {
		t.Fatalf("empty source: %+v", source.Info)
	}
	states := map[string]int{}
	for _, y := range []int32{sunriseSeaLevel >> 4, int32(sunriseRingY+sunriseRingR) >> 4} {
		for x := int32(-12); x <= 12; x++ {
			for z := int32(-12); z <= 12; z++ {
				section, err := source.Reader().Section(context.Background(), dispatcher.SectionPos{X: x, Y: y, Z: z})
				if err != nil {
					t.Fatal(err)
				}
				if section == nil {
					continue
				}
				for i := range section.Blocks.Indices {
					states[string(section.Blocks.Get(i))]++
				}
			}
		}
	}
	if states["minecraft:sea_lantern []"] == 0 || states["minecraft:cyan_stained_glass []"] == 0 {
		t.Fatal(states)
	}
}
