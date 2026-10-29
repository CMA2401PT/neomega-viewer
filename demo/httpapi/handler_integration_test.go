package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"neomega-viewer/blockstate"
	"neomega-viewer/demo/catalog"
	"neomega-viewer/demo/httpapi"
	"neomega-viewer/singleblock"
)

// 属性切换通过完整状态的 hash 验证。
func TestBrowserTransport(t *testing.T) {
	dir := os.Getenv("NEOMEGA_ASSETS_DIR")
	if dir == "" {
		t.Skip("set NEOMEGA_ASSETS_DIR to run the real resource transport check")
	}
	engine, err := singleblock.Open(singleblock.OpenOptions{TablesPath: "../../resources/geometry.gob"})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := catalog.Java()
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(httpapi.Options{Engine: engine, Catalog: entries, AssetsDir: dir, WebDir: t.TempDir()})
	call := func(method, path, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		if recorder.Code != status {
			t.Fatalf("%s %s: %d, want %d: %s", method, path, recorder.Code, status, recorder.Body.String())
		}
		return recorder
	}
	catalogResponse := call(http.MethodGet, "/api/blocks", "", 200)
	var listed struct {
		Version string          `json:"version"`
		Groups  []catalog.Group `json:"groups"`
	}
	if err := json.Unmarshal(catalogResponse.Body.Bytes(), &listed); err != nil || len(listed.Groups) != len(entries.Groups) || listed.Version != engine.Version() {
		t.Fatalf("catalog contract: %v", err)
	}
	stone := entries.ByName["minecraft:stone"][0]
	id := strconv.FormatUint(uint64(stone.ID), 10)
	call(http.MethodGet, "/api/states?name=minecraft:stone", "", 200)
	call(http.MethodGet, "/api/state?id="+id, "", 200)
	geometry := call(http.MethodGet, "/api/block?id="+id, "", 200)
	var packet singleblock.Packet
	if err := json.Unmarshal(geometry.Body.Bytes(), &packet); err != nil || packet.Geometry.Quads != 6 || packet.Binding.ID != stone.ID {
		t.Fatalf("geometry contract: %v, %+v", err, packet.Geometry)
	}
	for _, batch := range packet.Geometry.Batches {
		if len(batch.ShadeNormals) != len(batch.Positions) || len(batch.Normals) != len(batch.Positions) {
			t.Fatal("vertex attribute sizes differ")
		}
	}
	statesResponse := call(http.MethodGet, "/api/states?name=minecraft:oak_slab", "", 200)
	var states []blockstate.Binding
	if err := json.Unmarshal(statesResponse.Body.Bytes(), &states); err != nil {
		t.Fatal(err)
	}
	for _, slab := range states {
		body, _ := json.Marshal(map[string]any{"id": slab.ID})
		geometry = call(http.MethodPost, "/api/block", string(body), 200)
		if err := json.Unmarshal(geometry.Body.Bytes(), &packet); err != nil || packet.Binding.ID != slab.ID {
			t.Fatalf("state hash contract: %v", err)
		}
		bounds := packet.Geometry.Bounds
		switch packet.Binding.Java.Properties["type"] {
		case "bottom":
			if bounds.Min[1] != 0 || bounds.Max[1] != 0.5 {
				t.Fatalf("bottom slab bounds: %+v", bounds)
			}
		case "top":
			if bounds.Min[1] != 0.5 || bounds.Max[1] != 1 {
				t.Fatalf("top slab bounds: %+v", bounds)
			}
		case "double":
			if bounds.Min[1] != 0 || bounds.Max[1] != 1 {
				t.Fatalf("double slab bounds: %+v", bounds)
			}
		}
	}
	for id, binding := range entries.ByID {
		if binding.Java.Name == "minecraft:moving_piston" {
			call(http.MethodGet, "/api/block?id="+strconv.FormatUint(uint64(id), 10), "", 422)
			break
		}
	}
	png := call(http.MethodGet, "/assets/atlas.png", "", 200)
	if png.Header().Get("Content-Type") != "image/png" || !bytes.HasPrefix(png.Body.Bytes(), []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("atlas route did not serve the PNG")
	}
	notices := call(http.MethodGet, "/assets/THIRD_PARTY_NOTICES.txt", "", 200)
	if !bytes.Contains(notices.Body.Bytes(), []byte("Copyright")) {
		t.Fatal("resource notices were not served with the atlas")
	}
	call(http.MethodGet, "/api/block?id=-1", "", 400)
	call(http.MethodGet, "/api/state?id=4294967296", "", 400)
	call(http.MethodGet, "/api/states?name=absent", "", 404)
	call(http.MethodPost, "/api/block", "{", 400)
}
