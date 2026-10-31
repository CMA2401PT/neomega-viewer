package subchunk

import (
	"encoding/binary"
	"fmt"
	"math/bits"

	"celler/layer_storage"
)

const Size = 16
const Volume = Size * Size * Size
const IDsByteSize = Volume * 4

type Position = layer_storage.Position

func position(index int) Position {
	return Position{X: uint8(index >> 8), Y: uint8(index & 15), Z: uint8((index >> 4) & 15)}
}
func index(x, y, z int) int { return (x << 8) | (z << 4) | y }

// ParseIDs reads one portable block layer: exactly 4096 little-endian uint32
// IDs, in X-Z-Y order.
func ParseIDs(data []byte) (*layer_storage.Palette[uint32], error) {
	if len(data) != IDsByteSize {
		return nil, fmt.Errorf("block layer must contain %d bytes, got %d", IDsByteSize, len(data))
	}
	ids := make([]uint32, 0)
	lookup := map[uint32]int{}
	var indexes [Volume]int
	for i := range indexes {
		id := binary.LittleEndian.Uint32(data[i*4:])
		paletteIndex, found := lookup[id]
		if !found {
			paletteIndex = len(ids)
			lookup[id] = paletteIndex
			ids = append(ids, id)
		}
		indexes[i] = paletteIndex
	}
	layer := &layer_storage.Palette[uint32]{}
	layer.Fill(bits.Len(uint(len(ids)-1)), ids[0])
	layer.SetPalette(ids)
	if len(ids) > 1 {
		for i, pi := range indexes {
			layer.SetIndex(position(i), pi)
		}
	}
	return layer, nil
}

// EncodeIDs exports logical values, including the default-filled cells.
func EncodeIDs(layer *layer_storage.Palette[uint32]) []byte {
	result := make([]byte, IDsByteSize)
	for i := 0; i < Volume; i++ {
		binary.LittleEndian.PutUint32(result[i*4:], layer.Get(position(i)))
	}
	return result
}
