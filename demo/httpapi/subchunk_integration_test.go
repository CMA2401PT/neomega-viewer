package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"celler/layer_storage"
	"neomega-blocks/ir"
	blockmesh "neomega-blocks/presets/java/v1/geometry"
	"neomega-converter/fixer"
	"neomega-viewer/demo/httpapi"
	"neomega-viewer/demo/scenes"
	"neomega-viewer/demo/singleblock"
	"neomega-viewer/demo/subchunk"
	"neomega-viewer/mesher"
	"neomega-viewer/mesher/geometry"
	"neomega-viewer/splitlayer"
)

func TestSubchunkBinaryTransport(t *testing.T) {
	dir := os.Getenv("NEOMEGA_ASSETS_DIR")
	if dir == "" {
		t.Skip("set NEOMEGA_ASSETS_DIR for the binary subchunk transport check")
	}
	parser, err := blockmesh.Open(blockmesh.OpenOptions{TablesPath: "../../../neomega-blocks/presets/java/v1/geometry/geometry.nvtables"})
	if err != nil {
		t.Fatal(err)
	}
	stateFixer, err := fixer.New()
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(httpapi.Options{
		Engine:    singleblock.New(parser),
		Subchunks: mesher.NewMesher(parser.Tables),
		Fixer:     stateFixer,
		AssetsDir: dir,
		WebDir:    t.TempDir(),
	})
	call := func(method, path string, body []byte, want int) *httptest.ResponseRecorder {
		t.Helper()
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, httptest.NewRequest(method, path, bytes.NewReader(body)))
		if out.Code != want {
			t.Fatalf("%s %s -> %d: %s", method, path, out.Code, out.Body.String())
		}
		return out
	}
	response := call("GET", "/api/subchunks/solid/mesh", nil, 200)
	if response.Header().Get("Content-Type") != geometry.ContentType {
		t.Fatal("mesh is not binary")
	}
	mesh, err := geometry.UnmarshalMesh(bytes.NewReader(response.Body.Bytes()))
	if err != nil || len(mesh.Quads) != 6 {
		t.Fatalf("invalid binary geometry: %v, %d faces", err, len(mesh.Quads))
	}
	var stats mesher.Stats
	if err := json.Unmarshal([]byte(response.Header().Get("X-Neomega-Mesh-Stats")), &stats); err != nil || stats.InputQuads != 24576 || stats.VisibleQuads != 1536 || stats.OutputQuads != 6 {
		t.Fatalf("invalid pipeline stats: %v %+v", err, stats)
	}
	layer, err := demo.Build("solid")
	if err != nil {
		t.Fatal(err)
	}
	uploaded := call(http.MethodPost, "/api/subchunk", subchunk.EncodeIDs(layer), 200)
	if !bytes.Equal(uploaded.Body.Bytes(), response.Body.Bytes()) {
		t.Fatal("uploaded layer differs from direct palette input")
	}
	unmerged := call("GET", "/api/subchunks/solid/mesh?greedy=false", nil, 200)
	unmergedMesh, err := geometry.UnmarshalMesh(unmerged.Body)
	if err != nil || len(unmergedMesh.Quads) != 1536 {
		t.Fatalf("merge toggle: %v", err)
	}
	raw := call("GET", "/api/subchunks/solid/blocks", nil, 200)
	if !bytes.Equal(raw.Body.Bytes(), subchunk.EncodeIDs(layer)) {
		t.Fatal("download changed block IDs")
	}
	call("GET", "/api/subchunks/solid/mesh?cull=invalid", nil, 400)
	call("GET", "/api/subchunks/solid/mesh?greedy=invalid", nil, 400)
	call("GET", "/api/subchunks/missing/mesh", nil, 404)
	call("POST", "/api/subchunk", []byte{1, 2, 3}, 400)
	call("POST", "/api/subchunk", make([]byte, subchunk.IDsByteSize+1), 400)
}

func TestDirtyFenceRepairTransport(t *testing.T) {
	parser, err := blockmesh.Open(blockmesh.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stateFixer, err := fixer.New()
	if err != nil {
		t.Fatal(err)
	}
	engine := mesher.NewMesher(parser.Tables)
	handler := httpapi.New(httpapi.Options{
		Engine:    singleblock.New(parser),
		Subchunks: engine,
		Fixer:     stateFixer,
	})
	layer, err := demo.Build("fence_dirty")
	if err != nil {
		t.Fatal(err)
	}
	upload := subchunk.EncodeIDs(layer)
	meshBytes := func() []byte {
		t.Helper()
		var plain layer_storage.Plain[uint32]
		plain.CopyFrom(layer)
		result, err := engine.MeshSubchunk(plain.Data, mesher.Options{})
		if err != nil {
			t.Fatal(err)
		}
		var buffer bytes.Buffer
		if err := result.Mesh.Marshal(&buffer); err != nil {
			t.Fatal(err)
		}
		return buffer.Bytes()
	}
	rawMesh := meshBytes()
	fence := ir.IRBlock(`minecraft:oak_fence ["east"="false","north"="false","south"="false","west"="false"]`).Hash()
	setFence := func(x, z int, east, north, south, west bool) {
		t.Helper()
		position := layer_storage.At(x, 1, z)
		if layer.Get(position) != fence {
			t.Fatalf("fence at (%d,1,%d) is not the disconnected default", x, z)
		}
		layer.Set(position, ir.IRBlock(fmt.Sprintf(
			`minecraft:oak_fence ["east"="%t","north"="%t","south"="%t","west"="%t"]`,
			east,
			north,
			south,
			west,
		)).Hash())
	}
	// 独立写出预期连接，连向石头的端点也必须生成横杆。
	for x := 3; x <= 9; x++ {
		setFence(x, 3, true, false, false, x > 3)
	}
	for x := 3; x <= 7; x++ {
		setFence(x, 7, x < 7, false, x == 7, x > 3)
	}
	for z := 8; z <= 11; z++ {
		setFence(7, z, false, true, true, false)
	}
	for _, position := range [][2]int{{12, 7}, {12, 10}, {3, 12}} {
		setFence(position[0], position[1], false, false, false, false)
	}
	fixedMesh := meshBytes()
	for _, check := range []struct {
		method, path string
		body, want   []byte
	}{
		{http.MethodGet, "/api/subchunks/fence_dirty/mesh", nil, rawMesh},
		{http.MethodGet, "/api/subchunks/fence_dirty/mesh?fix=1", nil, fixedMesh},
		{http.MethodGet, "/api/subchunks/fence_dirty/mesh?fix=0", nil, rawMesh},
		{http.MethodPost, "/api/subchunk?fix=1", upload, fixedMesh},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(check.method, check.path, bytes.NewReader(check.body)))
		if response.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", check.path, response.Code, response.Body.String())
		}
		if !bytes.Equal(response.Body.Bytes(), check.want) {
			t.Fatalf("%s: geometry differs from expected fence connections", check.path)
		}
	}
}

func TestWaterloggedSceneAndUploadTransport(t *testing.T) {
	parser, err := blockmesh.Open(blockmesh.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stateFixer, err := fixer.New()
	if err != nil {
		t.Fatal(err)
	}
	engine := mesher.NewMesher(parser.Tables)
	handler := httpapi.New(httpapi.Options{
		Engine:    singleblock.New(parser),
		Subchunks: engine,
		Fixer:     stateFixer,
	})
	layer, err := demo.Build("waterlogged")
	if err != nil {
		t.Fatal(err)
	}
	upload := subchunk.EncodeIDs(layer)
	var plain layer_storage.Plain[uint32]
	plain.CopyFrom(layer)
	wetSlab := ir.IRBlock(`minecraft:oak_slab ["type"="bottom","waterlogged"="true"]`).Tidy().Hash()
	if plain.Get(layer_storage.At(3, 1, 3)) != wetSlab {
		t.Fatal("scene default properties discarded waterlogged")
	}
	for _, fix := range []string{"0", "1"} {
		layers := splitlayer.New().SplitSubchunk(plain.Data)
		if fix == "1" {
			layers[0] = stateFixer.Fix(layers[0], fixer.Neighbours{})
		}
		want, err := engine.MeshMultiLayerSubchunks(layers, mesher.Options{})
		if err != nil {
			t.Fatal(err)
		}
		var mesh bytes.Buffer
		if err := want.Mesh.Marshal(&mesh); err != nil {
			t.Fatal(err)
		}
		for _, request := range []struct {
			method string
			path   string
			body   []byte
		}{
			{http.MethodGet, "/api/subchunks/waterlogged/mesh?fix=" + fix, nil},
			{http.MethodPost, "/api/subchunk?fix=" + fix, upload},
		} {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(request.method, request.path, bytes.NewReader(request.body)))
			if response.Code != http.StatusOK {
				t.Fatalf("%s: %d %s", request.path, response.Code, response.Body.String())
			}
			if !bytes.Equal(response.Body.Bytes(), mesh.Bytes()) {
				t.Fatalf("%s: split layers or foreground repair changed", request.path)
			}
			var stats mesher.Stats
			if err := json.Unmarshal([]byte(response.Header().Get("X-Neomega-Mesh-Stats")), &stats); err != nil || stats != want.Stats {
				t.Fatalf("%s: stats=%+v, error=%v", request.path, stats, err)
			}
		}
	}
}
