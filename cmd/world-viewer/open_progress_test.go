package main

import (
	"bytes"
	"context"
	"encoding/json"
	nbt "neomega-nbt/presets/v1"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// 打开接口必须先发阶段事件，最终结果保留耗时；重复打开不伪造一次转换。
func TestOpenProgressAndReuse(t *testing.T) {
	file := filepath.Join(t.TempDir(), "tiny.schem")
	raw, err := nbt.EncodeToNBTBytes(map[string]any{"Version": int32(2), "Width": int16(1), "Height": int16(1), "Length": int16(1), "Palette": map[string]int32{"minecraft:stone": 0}, "BlockData": nbt.Array([]byte{0})}, nbt.OptEncodeToNBT{Encoding: nbt.EncodingBigEndian})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, raw, 0644); err != nil {
		t.Fatal(err)
	}
	source, err := OpenSource(context.Background(), SourceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	worlds := NewWorlds(source, nil)
	defer worlds.Close()
	handler := Handler(ServerOptions{Worlds: worlds})
	for _, reused := range []bool{false, true} {
		body, _ := json.Marshal(map[string]string{"path": file})
		request := httptest.NewRequest("POST", "/api/world", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/x-ndjson")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != 200 || !response.Flushed || response.Header().Get("Content-Type") != "application/x-ndjson" {
			t.Fatal(response)
		}
		decoder := json.NewDecoder(response.Body)
		stages := map[string]bool{}
		var info *Info
		for decoder.More() {
			var event openEvent
			if err = decoder.Decode(&event); err != nil {
				t.Fatal(err)
			}
			if event.Type == "error" {
				t.Fatal(event.Error)
			}
			if event.Progress != nil {
				stages[event.Progress.Stage] = true
			}
			if event.Type == "complete" {
				info = event.Info
			}
		}
		if info == nil || info.ReusedStructure != reused {
			t.Fatal(info)
		}
		if reused {
			if info.ConversionSeconds != nil || stages["reading"] {
				t.Fatal("复用时误报转换", info, stages)
			}
		} else {
			if info.ConversionSeconds == nil || *info.ConversionSeconds <= 0 {
				t.Fatal(info)
			}
			for _, stage := range []string{"reading", "arranging", "writing", "complete"} {
				if !stages[stage] {
					t.Fatal(stages)
				}
			}
		}
	}
	// 流已经发出时通过 error 事件报告失败，原来的世界仍可用。
	broken := filepath.Join(t.TempDir(), "bad.schem")
	os.WriteFile(broken, []byte("broken"), 0644)
	body, _ := json.Marshal(map[string]string{"path": broken})
	request := httptest.NewRequest("POST", "/api/world", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/x-ndjson")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !bytes.Contains(response.Body.Bytes(), []byte(`"type":"error"`)) || worlds.view().Info.Path != file {
		t.Fatal(response.Body.String())
	}
}
