// Package meshwire transports quad buffers with a small JSON material table and
// aligned little-endian numeric arrays. It is independent of HTTP and subchunks.
package meshwire

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"

	"neomega-viewer/geometry"
	"neomega-viewer/material"
)

const Version uint32 = 1
const HeaderSize = 44
const ContentType = "application/vnd.neomega.mesh"
const magic = "NVMS"

// Encode uses a fixed attribute order: position, normal, shadeNormal, UV, color,
// then uint32 indices. Only the material table contains JSON, not vertex data.
func Encode(mesh geometry.Buffers) ([]byte, error) {
	surfaces := make([]material.Surface, len(mesh.Batches))
	for i, batch := range mesh.Batches {
		surfaces[i] = batch.Surface
	}
	metadata, err := json.Marshal(surfaces)
	if err != nil {
		return nil, err
	}
	result := make([]byte, HeaderSize)
	copy(result, magic)
	binary.LittleEndian.PutUint32(result[4:], Version)
	binary.LittleEndian.PutUint32(result[8:], uint32(len(metadata)))
	binary.LittleEndian.PutUint32(result[12:], uint32(len(mesh.Batches)))
	binary.LittleEndian.PutUint32(result[16:], uint32(mesh.Quads))
	for i := 0; i < 3; i++ {
		binary.LittleEndian.PutUint32(result[20+i*4:], math.Float32bits(float32(mesh.Bounds.Min[i])))
		binary.LittleEndian.PutUint32(result[32+i*4:], math.Float32bits(float32(mesh.Bounds.Max[i])))
	}
	result = append(result, metadata...)
	for len(result)%4 != 0 {
		result = append(result, 0)
	}
	for _, batch := range mesh.Batches {
		count := len(batch.Positions) / 3
		if count%4 != 0 || len(batch.Positions) != count*3 || len(batch.Normals) != count*3 || len(batch.ShadeNormals) != count*3 || len(batch.Colors) != count*3 || len(batch.UVs) != count*2 || len(batch.Indices) != count/4*6 {
			return nil, fmt.Errorf("inconsistent quad buffer attribute lengths")
		}
		result = binary.LittleEndian.AppendUint32(result, uint32(count))
		result = binary.LittleEndian.AppendUint32(result, uint32(len(batch.Indices)))
		for _, attribute := range [][]float32{batch.Positions, batch.Normals, batch.ShadeNormals, batch.UVs, batch.Colors} {
			for _, value := range attribute {
				result = binary.LittleEndian.AppendUint32(result, math.Float32bits(value))
			}
		}
		for _, value := range batch.Indices {
			result = binary.LittleEndian.AppendUint32(result, value)
		}
	}
	return result, nil
}

// Decode reads the same portable layout for Go consumers. Browser consumers can
// instead view the aligned arrays directly in the received ArrayBuffer.
func Decode(data []byte) (geometry.Buffers, error) {
	var result geometry.Buffers
	if len(data) < HeaderSize || string(data[:4]) != magic {
		return result, fmt.Errorf("invalid NVMS header")
	}
	if version := binary.LittleEndian.Uint32(data[4:]); version != Version {
		return result, fmt.Errorf("unsupported NVMS version %d", version)
	}
	metadataLength := uint64(binary.LittleEndian.Uint32(data[8:]))
	batchCount := uint64(binary.LittleEndian.Uint32(data[12:]))
	result.Quads = int(binary.LittleEndian.Uint32(data[16:]))
	for i := 0; i < 3; i++ {
		result.Bounds.Min[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(data[20+i*4:])))
		result.Bounds.Max[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(data[32+i*4:])))
	}
	offset := uint64(HeaderSize) + metadataLength
	if offset > uint64(len(data)) {
		return result, fmt.Errorf("truncated material table")
	}
	var surfaces []material.Surface
	if err := json.Unmarshal(data[HeaderSize:offset], &surfaces); err != nil {
		return result, fmt.Errorf("material table: %w", err)
	}
	if uint64(len(surfaces)) != batchCount {
		return result, fmt.Errorf("material count differs from batch count")
	}
	offset = (offset + 3) &^ 3
	result.Batches = make([]geometry.Batch, 0, len(surfaces))
	quads := 0
	for _, surface := range surfaces {
		if offset+8 > uint64(len(data)) {
			return result, fmt.Errorf("truncated batch header")
		}
		vertices := uint64(binary.LittleEndian.Uint32(data[offset:]))
		indexes := uint64(binary.LittleEndian.Uint32(data[offset+4:]))
		offset += 8
		if vertices%4 != 0 || indexes != vertices/4*6 {
			return result, fmt.Errorf("batch is not a quad mesh")
		}
		if offset+vertices*14*4+indexes*4 > uint64(len(data)) {
			return result, fmt.Errorf("truncated vertex buffers")
		}
		readFloats := func(count uint64) []float32 {
			values := make([]float32, int(count))
			for i := range values {
				values[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[offset:]))
				offset += 4
			}
			return values
		}
		batch := geometry.Batch{Surface: surface}
		batch.Positions = readFloats(vertices * 3)
		batch.Normals = readFloats(vertices * 3)
		batch.ShadeNormals = readFloats(vertices * 3)
		batch.UVs = readFloats(vertices * 2)
		batch.Colors = readFloats(vertices * 3)
		batch.Indices = make([]uint32, int(indexes))
		for i := range batch.Indices {
			batch.Indices[i] = binary.LittleEndian.Uint32(data[offset:])
			offset += 4
			if uint64(batch.Indices[i]) >= vertices {
				return result, fmt.Errorf("index outside vertex buffer")
			}
		}
		quads += int(vertices / 4)
		result.Batches = append(result.Batches, batch)
	}
	if offset != uint64(len(data)) || quads != result.Quads {
		return result, fmt.Errorf("mesh length or quad count differs from header")
	}
	return result, nil
}
