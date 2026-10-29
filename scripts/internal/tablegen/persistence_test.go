package tablegen

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"neomega-viewer/demo/blockmesh"
	"neomega-viewer/mesher"
	"neomega-viewer/resources"
	"neomega-viewer/solidtable"
)

func TestPersistedTablesMatchBuild(t *testing.T) {
	dir := os.Getenv("NEOMEGA_ASSETS_DIR")
	if dir == "" {
		t.Skip("set NEOMEGA_ASSETS_DIR to compare generated and persisted tables")
	}
	builder, err := Open(OpenOptions{AssetsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	want, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	var solidBytes bytes.Buffer
	if err := want.Solid.Encode(&solidBytes); err != nil {
		t.Fatal(err)
	}
	embeddedSolid, err := resources.Files.ReadFile("geometry.nvsolid")
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"roundtrip": solidBytes.Bytes(), "embedded": embeddedSolid} {
		got, err := solidtable.Decode(bytes.NewReader(data))
		if err != nil || !maps.Equal(got, want.Solid) {
			t.Fatalf("%s solid table differs: error=%v", name, err)
		}
	}
	if len(want.Solid) != len(want.Index) {
		t.Fatal("solid table and geometry index contain different numbers of states")
	}
	path := filepath.Join(t.TempDir(), "geometry.nvtables")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = want.Encode(file)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("write table: %v; close: %v", err, closeErr)
	}
	metadata, err := json.Marshal(want.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".java.json", metadata, 0644); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		Name string
		Path string
	}{
		{Name: "roundtrip", Path: path},
		{Name: "committed", Path: "../../../resources/geometry.nvtables"},
		{Name: "embedded"},
	} {
		t.Run(check.Name, func(t *testing.T) {
			resolver, err := blockmesh.Open(blockmesh.OpenOptions{TablesPath: check.Path})
			if err != nil {
				t.Fatal(err)
			}
			got := resolver.Tables()
			// 空切片与 nil 的查询语义相同，按元素比较。
			if len(got.Index) != len(want.Index) || len(got.Shapes) != len(want.Shapes) {
				t.Fatalf("table sizes differ: states=%d/%d shapes=%d/%d", len(got.Index), len(want.Index), len(got.Shapes), len(want.Shapes))
			}
			for hash, ids := range want.Index {
				loaded, err := got.ModelIDs(hash)
				if err != nil || !slices.Equal(loaded, ids) {
					t.Fatalf("Java hash %d: IDs=%v, want %v, error=%v", hash, loaded, ids, err)
				}
			}
			for id, shape := range want.Shapes {
				loaded, err := got.Shape(mesher.ModelID(id))
				if err != nil {
					t.Fatal(err)
				}
				if loaded.CullGroup != shape.CullGroup || !slices.Equal(loaded.Quads, shape.Quads) {
					t.Fatalf("shape %d geometry differs", id)
				}
				for side, covers := range shape.Boundary {
					if !slices.Equal(loaded.Boundary[side], covers) {
						t.Fatalf("shape %d boundary %d differs", id, side)
					}
				}
			}
			if !maps.Equal(got.FullHeight, want.FullHeight) {
				t.Fatal("fluid full-height references differ")
			}
			if resolver.Metadata().Version != want.Version || !maps.Equal(resolver.Metadata().Renderers, want.Renderers) || !reflect.DeepEqual(resolver.Metadata().Skipped, want.Skipped) {
				t.Fatal("response metadata differs")
			}
			t.Logf("matched %d Java hashes, %d shapes, %d fluid references", len(want.Index), len(want.Shapes), len(want.FullHeight))
		})
	}
}
