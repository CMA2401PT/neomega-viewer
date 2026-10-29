package mesher

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"maps"
	"math"
	"math/bits"
	"slices"

	"neomega-assets/asset"
	"neomega-viewer/geometry"
	"neomega-viewer/material"
)

const tableMagic = "NVGT"
const tableVersion = 1

type tableHeader struct {
	Magic      [4]byte
	Version    uint16
	BlockWidth uint8
	Reserved   uint8
	Indexes    uint32
	Shapes     uint32
	FullHeight uint32
	Surfaces   uint32
	Vec3s      uint32
	Vec2s      uint32
}

type wireQuad struct {
	// 向量引用保留字典中的 float64 精度，避免改变裁切与合并判定。
	Positions   [4]uint32
	Normal      uint32
	ShadeNormal uint32
	UV          [4]uint32
	Color       [3]float32
	Surface     uint32
	CullFace    geometry.CullFace
}

type wireBoundary struct {
	Quad uint32
	Min  geometry.Vec2
	Max  geometry.Vec2
}

func blockWidth[B BlockID]() uint8 {
	return uint8(bits.Len64(uint64(^B(0))) / 8)
}

func (tables *Tables[B]) Encode(w io.Writer) error {
	// 共享重复向量和材质，在保留数值精度的同时减小文件。
	vec3s := []geometry.Vec3{}
	vec2s := []geometry.Vec2{}
	vec3IDs := map[[3]uint64]uint32{}
	vec2IDs := map[[2]uint64]uint32{}
	add3 := func(value geometry.Vec3) {
		key := vec3Key(value)
		if _, ok := vec3IDs[key]; !ok {
			vec3IDs[key] = uint32(len(vec3s))
			vec3s = append(vec3s, value)
		}
	}
	add2 := func(value geometry.Vec2) {
		key := vec2Key(value)
		if _, ok := vec2IDs[key]; !ok {
			vec2IDs[key] = uint32(len(vec2s))
			vec2s = append(vec2s, value)
		}
	}
	surfaces := []material.Surface{}
	surfaceIDs := map[material.Surface]uint32{}
	for _, shape := range tables.Shapes {
		for _, quad := range shape.Quads {
			for _, value := range quad.Positions {
				add3(value)
			}
			add3(quad.Normal)
			add3(quad.ShadeNormal)
			for _, value := range quad.UV {
				add2(value)
			}
			if _, ok := surfaceIDs[quad.Surface]; !ok {
				surfaceIDs[quad.Surface] = uint32(len(surfaces))
				surfaces = append(surfaces, quad.Surface)
			}
		}
	}
	buffer := bufio.NewWriter(w)
	writer := tableWriter{w: buffer}
	writer.write(tableHeader{
		Magic:      [4]byte{'N', 'V', 'G', 'T'},
		Version:    tableVersion,
		BlockWidth: blockWidth[B](),
		Indexes:    uint32(len(tables.Index)),
		Shapes:     uint32(len(tables.Shapes)),
		FullHeight: uint32(len(tables.FullHeight)),
		Surfaces:   uint32(len(surfaces)),
		Vec3s:      uint32(len(vec3s)),
		Vec2s:      uint32(len(vec2s)),
	})
	for _, surface := range surfaces {
		writer.text(string(surface.Texture))
		writer.text(string(surface.Mode))
		writer.write(surface.Region)
		writer.write(surface.DoubleSided)
		writer.write(surface.Unlit)
		writer.write(surface.Opacity)
	}
	writer.write(vec3s)
	writer.write(vec2s)
	// 排序使同一张表的编码稳定，不依赖 map 遍历顺序。
	for _, id := range slices.Sorted(maps.Keys(tables.Index)) {
		if blockWidth[B]() == 4 {
			writer.write(uint32(id))
		} else {
			writer.write(uint64(id))
		}
		writer.write(uint32(len(tables.Index[id])))
		writer.write(tables.Index[id])
	}
	for _, shape := range tables.Shapes {
		writer.write(shape.CullGroup)
		writer.write(uint32(len(shape.Quads)))
		for _, quad := range shape.Quads {
			wire := wireQuad{
				Normal:      vec3IDs[vec3Key(quad.Normal)],
				ShadeNormal: vec3IDs[vec3Key(quad.ShadeNormal)],
				Color:       quad.Color,
				Surface:     surfaceIDs[quad.Surface],
				CullFace:    quad.CullFace,
			}
			for i := range quad.Positions {
				wire.Positions[i] = vec3IDs[vec3Key(quad.Positions[i])]
			}
			for i := range quad.UV {
				wire.UV[i] = vec2IDs[vec2Key(quad.UV[i])]
			}
			writer.write(wire)
		}
		for _, boundary := range shape.Boundary {
			writer.write(uint32(len(boundary)))
			for _, rect := range boundary {
				writer.write(wireBoundary{Quad: uint32(rect.Quad), Min: rect.Min, Max: rect.Max})
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(tables.FullHeight)) {
		writer.write(id)
		writer.write(tables.FullHeight[id])
	}
	if writer.err != nil {
		return writer.err
	}
	return buffer.Flush()
}

func DecodeTables[B BlockID](r io.Reader) (*Tables[B], error) {
	reader := tableReader{r: r}
	var header tableHeader
	reader.read(&header)
	if reader.err != nil {
		return nil, reader.err
	}
	if string(header.Magic[:]) != tableMagic {
		return nil, fmt.Errorf("invalid NVGT magic")
	}
	if header.Version != tableVersion {
		return nil, fmt.Errorf("unsupported NVGT version %d", header.Version)
	}
	if header.BlockWidth != blockWidth[B]() {
		return nil, fmt.Errorf("NVGT block ID width %d does not match requested width %d", header.BlockWidth, blockWidth[B]())
	}
	surfaces := make([]material.Surface, header.Surfaces)
	for i := range surfaces {
		surface := &surfaces[i]
		surface.Texture = asset.ID(reader.text())
		surface.Mode = material.Mode(reader.text())
		reader.read(&surface.Region)
		reader.read(&surface.DoubleSided)
		reader.read(&surface.Unlit)
		reader.read(&surface.Opacity)
	}
	vec3s := make([]geometry.Vec3, header.Vec3s)
	vec2s := make([]geometry.Vec2, header.Vec2s)
	reader.read(vec3s)
	reader.read(vec2s)
	tables := &Tables[B]{
		Index:      make(map[B][]ModelID, header.Indexes),
		Shapes:     make([]Shape, header.Shapes),
		FullHeight: make(map[ModelID]ModelID, header.FullHeight),
	}
	for range header.Indexes {
		var id B
		if header.BlockWidth == 4 {
			var value uint32
			reader.read(&value)
			id = B(value)
		} else {
			var value uint64
			reader.read(&value)
			id = B(value)
		}
		ids := make([]ModelID, reader.count())
		reader.read(ids)
		tables.Index[id] = ids
	}
	for i := range tables.Shapes {
		shape := &tables.Shapes[i]
		reader.read(&shape.CullGroup)
		shape.Quads = make([]geometry.Quad, reader.count())
		for j := range shape.Quads {
			var quad wireQuad
			reader.read(&quad)
			if reader.err != nil {
				return nil, reader.err
			}
			decoded := geometry.Quad{
				Normal:      vec3s[quad.Normal],
				ShadeNormal: vec3s[quad.ShadeNormal],
				Color:       quad.Color,
				Surface:     surfaces[quad.Surface],
				CullFace:    quad.CullFace,
			}
			for i, id := range quad.Positions {
				decoded.Positions[i] = vec3s[id]
			}
			for i, id := range quad.UV {
				decoded.UV[i] = vec2s[id]
			}
			shape.Quads[j] = decoded
		}
		for side := range shape.Boundary {
			boundary := make([]geometry.BoundaryRect, reader.count())
			for j := range boundary {
				var rect wireBoundary
				reader.read(&rect)
				boundary[j] = geometry.BoundaryRect{Quad: int(rect.Quad), Min: rect.Min, Max: rect.Max}
			}
			shape.Boundary[side] = boundary
		}
	}
	for range header.FullHeight {
		var id, full ModelID
		reader.read(&id)
		reader.read(&full)
		tables.FullHeight[id] = full
	}
	if reader.err != nil {
		return nil, reader.err
	}
	return tables, nil
}

type tableWriter struct {
	w   io.Writer
	err error
}

func (writer *tableWriter) write(value any) {
	// 只保留首个 I/O 错误，字段布局不被重复错误分支打断。
	if writer.err == nil {
		writer.err = binary.Write(writer.w, binary.LittleEndian, value)
	}
}

func (writer *tableWriter) text(value string) {
	writer.write(uint32(len(value)))
	if writer.err == nil {
		_, writer.err = io.WriteString(writer.w, value)
	}
}

type tableReader struct {
	r   io.Reader
	err error
}

func (reader *tableReader) read(value any) {
	if reader.err == nil {
		reader.err = binary.Read(reader.r, binary.LittleEndian, value)
	}
}

func (reader *tableReader) count() uint32 {
	var count uint32
	reader.read(&count)
	return count
}

func (reader *tableReader) text() string {
	data := make([]byte, reader.count())
	if reader.err == nil {
		_, reader.err = io.ReadFull(reader.r, data)
	}
	return string(data)
}

func vec3Key(value geometry.Vec3) [3]uint64 {
	// 按位去重保留负零等原始浮点表示。
	return [3]uint64{math.Float64bits(value[0]), math.Float64bits(value[1]), math.Float64bits(value[2])}
}

func vec2Key(value geometry.Vec2) [2]uint64 {
	return [2]uint64{math.Float64bits(value[0]), math.Float64bits(value[1])}
}
