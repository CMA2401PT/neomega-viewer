package worldview

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"image/png"
	"net/http/httptest"
	"testing"

	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer/minimap"
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
	for _, y := range []int32{int32(backdrop.SeaLevel) >> 4, int32(backdrop.Center[1]+backdrop.Radius) >> 4} {
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

func TestSunriseMinimap(t *testing.T) {
	// 默认场景同时提供区块占用图和含透明材质的体素区域。
	src, err := OpenSource(context.Background(), SourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	worlds := NewWorlds(src, nil, "test")
	defer worlds.Close()
	handler := Handler(ServerOptions{Worlds: worlds})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/world", nil))
	var info Info
	if err := json.Unmarshal(response.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Loaded || info.Backdrop == nil || !info.Minimap {
		t.Fatal(info)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/minimap/index?world=1", nil))
	var index minimap.Index
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Regions) == 0 || len(index.Overview) == 0 {
		t.Fatal(index)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/minimap/tile?world=1&kind=overview&x=0&z=0", nil))
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	img, err := png.Decode(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 128 || img.Bounds().Dy() != 128 {
		t.Fatal(img.Bounds())
	}
	visible, transparent := false, false
	for y := range 128 {
		for x := range 128 {
			_, _, _, alpha := img.At(x, y).RGBA()
			visible = visible || alpha != 0
			transparent = transparent || alpha == 0
		}
	}
	if !visible || !transparent {
		t.Fatal("默认场景应同时含可见方块与透明空气")
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/minimap/region?world=1&x=0&z=0", nil))
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var header struct {
		Format     string `json:"format"`
		Origin     [3]int `json:"origin"`
		Size       [3]int `json:"size"`
		BrickBytes int    `json:"brickBytes"`
	}
	data := response.Body.Bytes()
	length := binary.LittleEndian.Uint32(data)
	if err := json.Unmarshal(data[4:4+length], &header); err != nil {
		t.Fatal(err)
	}
	if header.Format != "brickmap-2" || header.Origin != [3]int{0, -64, 0} || header.Size != [3]int{128, 512, 128} || header.BrickBytes == 0 {
		t.Fatal(header)
	}
}
