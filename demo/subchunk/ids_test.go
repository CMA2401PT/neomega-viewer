package subchunk_test

import (
	"encoding/binary"
	"testing"

	"celler"
	"neomega-viewer/demo/subchunk"
)

func TestPortableLayerCoordinatesAndDefaultFilledCells(t *testing.T) {
	data := make([]byte, subchunk.IDsByteSize)
	for i := 0; i < 4096; i++ {
		binary.LittleEndian.PutUint32(data[i*4:], 42)
	}
	uniform, err := subchunk.ParseIDs(data)
	if err != nil {
		t.Fatal(err)
	}
	if uniform.Level != 0 || uniform.Get(celler.ToSubChunkBlockPos(15, 15, 15)) != 42 {
		t.Fatal("default-filled cells lost")
	}
	binary.LittleEndian.PutUint32(data[((3<<8)|(9<<4)|7)*4:], 1234567890)
	layer, err := subchunk.ParseIDs(data)
	if err != nil {
		t.Fatal(err)
	}
	if layer.Get(celler.ToSubChunkBlockPos(3, 7, 9)) != 1234567890 || layer.Get(celler.ToSubChunkBlockPos(3, 9, 7)) != 42 {
		t.Fatal("XZY order lost")
	}
	encoded := subchunk.EncodeIDs(layer)
	if string(encoded) != string(data) {
		t.Fatal("logical layer values changed")
	}
	if _, err := subchunk.ParseIDs(data[:len(data)-1]); err == nil {
		t.Fatal("short layer accepted")
	}
}
