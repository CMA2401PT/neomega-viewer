package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"neomega-viewer/demo/blockmesh"
	"neomega-viewer/demo/httpapi"
	"neomega-viewer/demo/scenes"
	"neomega-viewer/demo/singleblock"
	"neomega-viewer/demo/subchunk"
	"neomega-viewer/mesher"
	"neomega-viewer/mesher/geometry"
)

func TestSubchunkBinaryTransport(t *testing.T) {
	dir := os.Getenv("NEOMEGA_ASSETS_DIR")
	if dir == "" {
		t.Skip("set NEOMEGA_ASSETS_DIR for the binary subchunk transport check")
	}
	parser, err := blockmesh.Open(blockmesh.OpenOptions{TablesPath: "../../resources/geometry.nvtables"})
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.New(httpapi.Options{Engine: singleblock.New(parser), Subchunks: mesher.NewMesher(parser.Tables()), AssetsDir: dir, WebDir: t.TempDir()})
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
