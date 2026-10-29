package solidtable

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"maps"
	"slices"
)

type Table map[uint32]uint8

// 位序为 west/east/down/up/north/south = bit0..bit5，便于直接查询邻居朝向当前格的一面。

type header struct {
	Magic   [4]byte
	Version uint32
	Count   uint32
}

type entry struct {
	Hash  uint32
	Solid uint8
}

func (table Table) Encode(w io.Writer) error {
	buffer := bufio.NewWriter(w)
	if err := binary.Write(buffer, binary.LittleEndian, header{
		Magic:   [4]byte{'N', 'V', 'S', 'T'},
		Version: 1,
		Count:   uint32(len(table)),
	}); err != nil {
		return err
	}
	// 固定记录顺序，使重新生成的文件可直接比较。
	for _, hash := range slices.Sorted(maps.Keys(table)) {
		if err := binary.Write(buffer, binary.LittleEndian, entry{Hash: hash, Solid: table[hash]}); err != nil {
			return err
		}
	}
	return buffer.Flush()
}

func Decode(r io.Reader) (Table, error) {
	buffer := bufio.NewReader(r)
	var head header
	if err := binary.Read(buffer, binary.LittleEndian, &head); err != nil {
		return nil, err
	}
	if string(head.Magic[:]) != "NVST" {
		return nil, fmt.Errorf("invalid NVST magic")
	}
	if head.Version != 1 {
		return nil, fmt.Errorf("unsupported NVST version %d", head.Version)
	}
	table := make(Table, head.Count)
	for range head.Count {
		var item entry
		if err := binary.Read(buffer, binary.LittleEndian, &item); err != nil {
			return nil, err
		}
		table[item.Hash] = item.Solid
	}
	return table, nil
}
