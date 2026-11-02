package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"celler/layer_storage"
	"neomega-viewer-core/fixer/java-postprocess"
	"neomega-viewer-core/viewer/dispatcher"
	jmodel "neomega-viewer-core/viewer/geometry/java_block_model"
	"neomega-viewer-core/viewer/merger"
	"neomega-viewer-core/viewer/mesher"
	"neomega-viewer-core/viewer/mesher/geometry"
	"neomega-viewer-core/viewer/mesher/meshing"
	blockmesh "neomega-viewer-core/viewer/resources"
	"neomega-viewer/demo/httpapi"
	"neomega-viewer/demo/scenes"
	"neomega-viewer/demo/singleblock"
	"neomega-viewer/demo/subchunk"
)

func TestBlockEntitiesFrame(t *testing.T) {
	parser, err := blockmesh.Open(blockmesh.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stateFixer, err := fixer.New()
	if err != nil {
		t.Fatal(err)
	}
	atlas := jmodel.Atlas{Textures: map[jmodel.ResourceID]jmodel.AtlasTexture{}}
	for i, name := range []string{"decorated_pot_side", "angler_pottery_pattern", "archer_pottery_pattern", "arms_up_pottery_pattern", "blade_pottery_pattern", "prize_pottery_pattern"} {
		atlas.Textures[jmodel.ResourceID("minecraft:entity/decorated_pot/"+name)] = jmodel.AtlasTexture{
			U: float64(i) / 16, V: .5, SU: 1.0 / 16, SV: 1.0 / 16, Frames: 1,
		}
	}
	pipeline := httpapi.NewFramePipeline(parser.Tables, atlas)
	handler := httpapi.New(httpapi.Options{
		Engine:    singleblock.New(parser),
		Subchunks: mesher.NewMesher(parser.Tables),
		Frames:    pipeline,
		Fixer:     stateFixer,
	})
	scene, err := demo.BuildWithNBT("block_entities")
	if err != nil {
		t.Fatal(err)
	}
	var plain layer_storage.Plain[uint32]
	plain.CopyFrom(scene.Blocks)
	for _, query := range []string{"", "?fix=1", "?cull=false&greedy=false&fix=1"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/subchunks/block_entities/frame"+query, nil))
		if response.Code != 200 || response.Header().Get("Content-Type") != merger.ContentType {
			t.Fatalf("frame response: %d %s", response.Code, response.Body.String())
		}
		frame, err := merger.Unmarshal(bytes.NewReader(response.Body.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		if len(frame.Effects) != 1 || frame.Effects[0].Effect != "enchanting_table_book" || frame.Effects[0].Pos != ([3]uint8{3, 1, 3}) || len(frame.Actors) != 6 {
			t.Fatalf("frame contents: effects=%+v actors=%d", frame.Effects, len(frame.Actors))
		}
		textures := map[string]int{}
		for _, quad := range frame.Mesh.Quads {
			textures[quad.Surface.Texture]++
			if strings.Contains(quad.Surface.Texture, "decorated_pot") {
				t.Fatal("pot geometry remained in mesher payload")
			}
		}
		if textures["minecraft:block/water_still"] == 0 || textures["minecraft:block/enchanting_table_top"] == 0 {
			t.Fatalf("water or enchanting table disappeared: %v", textures)
		}
		quads := 0
		for _, actor := range frame.Actors {
			quads += len(actor.Mesh.Quads)
			if len(actor.Mesh.Quads) != 18 {
				t.Fatalf("pot at %v has %d quads", actor.Pos, len(actor.Mesh.Quads))
			}
		}
		options, input := dispatcher.Options{}, plain.Data
		if strings.Contains(query, "fix=1") {
			input = fixedHashes(t, stateFixer, input)
		}
		if strings.Contains(query, "cull=false") {
			options.Mesh.Meshing = meshing.Options{DisableCulling: true, DisableGreedy: true}
		}
		states, err := dispatcher.BlocksFromHashes(input)
		if err != nil {
			t.Fatal(err)
		}
		direct, err := pipeline.Build(
			dispatcher.Section{Blocks: states, Entities: scene.NBT},
			dispatcher.Neighbours{},
			options,
		)
		if err != nil {
			t.Fatal(err)
		}
		var encoded bytes.Buffer
		if err := direct.Frame.Marshal(&encoded); err != nil || !bytes.Equal(encoded.Bytes(), response.Body.Bytes()) {
			t.Fatalf("HTTP options differ from pipeline: %v", err)
		}
		var stats mesher.Stats
		if err := json.Unmarshal([]byte(response.Header().Get("X-Neomega-Mesh-Stats")), &stats); err != nil || stats != direct.Stats {
			t.Fatalf("stats differ: %+v, %v", stats, err)
		}
		t.Logf("%s: NVFR=%d bytes, static=%d quads textures=%v, effects=%d, actors=%d/%d quads", query, response.Body.Len(), len(frame.Mesh.Quads), textures, len(frame.Effects), len(frame.Actors), quads)
	}
	// 上传不携带 NBT，仍经同一 pipeline；书消息和无纹样 actor 都应保留。
	for _, query := range []string{"", "?fix=1", "?cull=false&greedy=false&fix=1"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("POST", "/api/subchunk/frame"+query, bytes.NewReader(subchunk.EncodeIDs(scene.Blocks))))
		if response.Code != 200 || response.Header().Get("Content-Type") != merger.ContentType {
			t.Fatalf("upload frame response: %d %s", response.Code, response.Body.String())
		}
		options, input := dispatcher.Options{}, plain.Data
		if strings.Contains(query, "fix=1") {
			input = fixedHashes(t, stateFixer, input)
		}
		if strings.Contains(query, "cull=false") {
			options.Mesh.Meshing = meshing.Options{DisableCulling: true, DisableGreedy: true}
		}
		states, err := dispatcher.BlocksFromHashes(input)
		if err != nil {
			t.Fatal(err)
		}
		direct, err := pipeline.Build(
			dispatcher.Section{Blocks: states, Entities: nil},
			dispatcher.Neighbours{},
			options,
		)
		if err != nil {
			t.Fatal(err)
		}
		var encoded bytes.Buffer
		if err := direct.Frame.Marshal(&encoded); err != nil || !bytes.Equal(encoded.Bytes(), response.Body.Bytes()) {
			t.Fatalf("uploaded frame differs from pipeline: %v", err)
		}
		frame, err := merger.Unmarshal(bytes.NewReader(response.Body.Bytes()))
		if err != nil || len(frame.Actors) != 6 || len(frame.Effects) != 1 {
			t.Fatalf("uploaded frame contents: %v", err)
		}
		var stats mesher.Stats
		if err := json.Unmarshal([]byte(response.Header().Get("X-Neomega-Mesh-Stats")), &stats); err != nil || stats != direct.Stats {
			t.Fatalf("upload stats differ: %+v, %v", stats, err)
		}
	}
	for _, test := range []struct {
		path string
		body []byte
	}{
		{"/api/subchunk/frame", []byte{1, 2, 3}},
		{"/api/subchunk/frame", make([]byte, subchunk.IDsByteSize+1)},
		{"/api/subchunk/frame?cull=invalid", subchunk.EncodeIDs(scene.Blocks)},
		{"/api/subchunk/frame?greedy=invalid", subchunk.EncodeIDs(scene.Blocks)},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("POST", test.path, bytes.NewReader(test.body)))
		if response.Code != 400 {
			t.Fatalf("%s -> %d, want 400", test.path, response.Code)
		}
	}
	// 原 mesh 入口仍输出完整静态陶罐，调用者可继续使用旧协议。
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/subchunks/block_entities/mesh", nil))
	mesh, err := geometry.UnmarshalMesh(response.Body)
	if response.Code != 200 || err != nil {
		t.Fatalf("legacy mesh: %d %v", response.Code, err)
	}
	potFaces := 0
	for _, quad := range mesh.Quads {
		if strings.Contains(quad.Surface.Texture, "decorated_pot") {
			potFaces++
		}
	}
	if potFaces == 0 {
		t.Fatal("legacy mesh unexpectedly routed pot to actors")
	}
	for _, test := range []struct {
		path string
		code int
	}{
		{"/api/subchunks/block_entities/frame?cull=invalid", 400},
		{"/api/subchunks/block_entities/frame?greedy=invalid", 400},
		{"/api/subchunks/missing/frame", 404},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", test.path, nil))
		if response.Code != test.code {
			t.Fatalf("%s -> %d, want %d", test.path, response.Code, test.code)
		}
	}
}
