package geometry

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"

	"neomega-viewer/mesher/material"
)

const Version uint32 = 1
const HeaderSize = 44
const ContentType = "application/vnd.neomega.mesh"
const magic = "NVMS"

func (mesh Mesh) Marshal(w io.Writer) error {
	// NVMS 按材质和属性排列，所有面必须一起分批后再写出。
	meshBuffers := mesh.Buffers()
	surfaces := make([]material.Surface, len(meshBuffers.Batches))
	for i, batch := range meshBuffers.Batches {
		surfaces[i] = batch.Surface
	}
	metadata, err := json.Marshal(surfaces)
	if err != nil {
		return err
	}
	header := make([]byte, HeaderSize)
	copy(header, magic)
	binary.LittleEndian.PutUint32(header[4:], Version)
	binary.LittleEndian.PutUint32(header[8:], uint32(len(metadata)))
	binary.LittleEndian.PutUint32(header[12:], uint32(len(meshBuffers.Batches)))
	binary.LittleEndian.PutUint32(header[16:], uint32(meshBuffers.Quads))
	for i := 0; i < 3; i++ {
		binary.LittleEndian.PutUint32(header[20+i*4:], math.Float32bits(float32(meshBuffers.Bounds.Min[i])))
		binary.LittleEndian.PutUint32(header[32+i*4:], math.Float32bits(float32(meshBuffers.Bounds.Max[i])))
	}
	buffer := bufio.NewWriter(w)
	if _, err := buffer.Write(header); err != nil {
		return err
	}
	if _, err := buffer.Write(metadata); err != nil {
		return err
	}
	padding := make([]byte, (4-len(metadata)%4)%4)
	if _, err := buffer.Write(padding); err != nil {
		return err
	}
	for _, batch := range meshBuffers.Batches {
		counts := [2]uint32{uint32(len(batch.Positions) / 3), uint32(len(batch.Indices))}
		if err := binary.Write(buffer, binary.LittleEndian, counts); err != nil {
			return err
		}
		for _, attribute := range [][]float32{batch.Positions, batch.Normals, batch.ShadeNormals, batch.UVs, batch.Colors} {
			if err := binary.Write(buffer, binary.LittleEndian, attribute); err != nil {
				return err
			}
		}
		if err := binary.Write(buffer, binary.LittleEndian, batch.Indices); err != nil {
			return err
		}
	}
	return buffer.Flush()
}

func UnmarshalMesh(r io.Reader) (Mesh, error) {
	// NVMS 只保存绘制精度和材质分批后的面序，不是烘焙数据的存档。
	data, err := io.ReadAll(r)
	if err != nil {
		return Mesh{}, err
	}
	buffers, err := decodeBuffers(data)
	if err != nil {
		return Mesh{}, err
	}
	mesh := Mesh{Quads: make([]Quad, 0, buffers.Quads)}
	for _, batch := range buffers.Batches {
		for base := 0; base < len(batch.Positions)/3; base += 4 {
			quad := Quad{Surface: batch.Surface}
			for axis := 0; axis < 3; axis++ {
				quad.Normal[axis] = float64(batch.Normals[base*3+axis])
				quad.ShadeNormal[axis] = float64(batch.ShadeNormals[base*3+axis])
				quad.Color[axis] = batch.Colors[base*3+axis]
			}
			for i := 0; i < 4; i++ {
				for axis := 0; axis < 3; axis++ {
					quad.Positions[i][axis] = float64(batch.Positions[(base+i)*3+axis])
				}
				for axis := 0; axis < 2; axis++ {
					quad.UV[i][axis] = float64(batch.UVs[(base+i)*2+axis])
				}
			}
			mesh.Quads = append(mesh.Quads, quad)
		}
	}
	return mesh, nil
}

func (quad Quad) Marshal(w io.Writer) error {
	// 单面复用 NVMS，避免引入另一种独立格式。
	return (Mesh{Quads: []Quad{quad}}).Marshal(w)
}

func UnmarshalQuad(r io.Reader) (Quad, error) {
	mesh, err := UnmarshalMesh(r)
	if err != nil {
		return Quad{}, err
	}
	if len(mesh.Quads) != 1 {
		return Quad{}, fmt.Errorf("expected one quad, got %d", len(mesh.Quads))
	}
	return mesh.Quads[0], nil
}

// Decode reads the same portable layout for Go consumers. Browser consumers can
// instead view the aligned arrays directly in the received ArrayBuffer.
func decodeBuffers(data []byte) (Buffers, error) {
	var result Buffers
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
	result.Batches = make([]Batch, 0, len(surfaces))
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
		batch := Batch{Surface: surface}
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
