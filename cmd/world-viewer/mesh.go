package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"

	"celler"
	"celler/java"
	"celler/layer_storage"
	core "neomega-blocks/geometry"
	jmodel "neomega-blocks/geometry/java_block_model"
	"neomega-blocks/geometry/material"
	"neomega-blocks/ir"
	nbt "neomega-nbt/presets/v1"
	"neomega-viewer/blockactor"
	"neomega-viewer/blockactor/decoratedpot"
	"neomega-viewer/effector"
	"neomega-viewer/merger"
	"neomega-viewer/mesher"
	"neomega-viewer/splitlayer"
)

const maxColumnBytes = 24 << 20

type chunkKey struct{ X, Z, MinY, MaxY int32 }

// RenderIssue retains the source blockstate before layer splitting. An empty
// blockstate identifies a column-level issue; Position is then the column origin.
type RenderIssue struct {
	Position   [3]int32 `json:"position"`
	Blockstate string   `json:"blockstate"`
	Reason     string   `json:"reason"`
	Detail     string   `json:"detail,omitempty"`
}

type prepared struct {
	layers  [][]uint32
	blocks  *layer_storage.Palette[ir.IRBlock]
	unknown int
	issues  []RenderIssue
	empty   bool
}

// Engine is immutable after creation. Request workers own all temporary arrays.
type Engine struct {
	tables       *core.Tables[uint32]
	mesher       *mesher.Mesher[uint32]
	missing, air uint32
	actor        *blockactor.Dispatcher
}

func NewEngine(tables *core.Tables[uint32], atlas *jmodel.Atlas) (*Engine, error) {
	copyTables := &core.Tables[uint32]{Index: maps.Clone(tables.Index), Shapes: append([]core.Shape(nil), tables.Shapes...), FullHeight: tables.FullHeight}
	stone, err := tables.ModelIDs(ir.IRBlock("minecraft:stone []").Hash())
	if err != nil || len(stone) == 0 {
		return nil, fmt.Errorf("geometry table requires stone for missing-state marker")
	}
	missing := ^uint32(0)
	for {
		if _, exists := copyTables.Index[missing]; !exists {
			break
		}
		missing--
	}
	for _, id := range stone {
		shape := tables.Shapes[id]
		shape.Quads = append([]core.Quad(nil), shape.Quads...)
		shape.CullGroup = 0
		for i := range shape.Quads {
			shape.Quads[i].Surface.Unlit = true
			shape.Quads[i].Color = [3]float32{1, 0, 1}
		}
		copyTables.Index[missing] = append(copyTables.Index[missing], core.ModelID(len(copyTables.Shapes)))
		copyTables.Shapes = append(copyTables.Shapes, shape)
	}
	e := &Engine{tables: copyTables, mesher: mesher.NewMesher(copyTables), missing: missing, air: ir.IRBlock("minecraft:air []").Hash()}
	if atlas != nil {
		e.actor = blockactor.New(decoratedpot.New(copyTables, func(texture string) (material.Surface, error) {
			entry, ok := atlas.Textures[jmodel.ResourceID(texture)]
			if !ok {
				return material.Surface{}, fmt.Errorf("missing actor texture %s", texture)
			}
			return material.Surface{Texture: texture, Region: [4]float32{float32(entry.U), float32(entry.V), float32(entry.SU), float32(entry.SV)}, Mode: material.Opaque, Opacity: 1, Frames: entry.Frames, FrameTime: entry.FrameTime}, nil
		}))
	}
	return e, nil
}
func (e *Engine) prepare(s *java.Section) *prepared {
	p := &prepared{blocks: s.Blocks, empty: true}
	if s.Blocks == nil {
		return p
	}
	palette := s.Blocks.GetUsedPalette()
	parts := make([][]uint32, len(palette))
	unknown := make([]bool, len(palette))
	visible := make([]bool, len(palette))
	depth := 1
	for i, block := range palette {
		parts[i] = splitlayer.SplitBlock(block)
		for j, id := range parts[i] {
			models, ok := e.tables.Index[id]
			if !ok {
				parts[i][j] = e.missing
				unknown[i] = true
				visible[i] = true
			} else if len(models) > 0 {
				visible[i] = true
			}
		}
		depth = max(depth, len(parts[i]))
	}
	p.layers = make([][]uint32, depth)
	for i := range p.layers {
		p.layers[i] = make([]uint32, 4096)
		for j := range p.layers[i] {
			p.layers[i][j] = e.air
		}
	}
	reported := make([]bool, len(palette))
	for i := 0; i < 4096; i++ {
		index := s.Blocks.GetIndex(layer_storage.At(i>>8, i&15, i>>4&15))
		for layer, id := range parts[index] {
			p.layers[layer][i] = id
		}
		if unknown[index] {
			p.unknown++
			if !reported[index] {
				reported[index] = true
				p.issues = append(p.issues, RenderIssue{
					Position:   [3]int32{int32(i >> 8), s.Y*16 + int32(i&15), int32(i >> 4 & 15)},
					Blockstate: string(palette[index]),
					Reason:     "missing_model",
				})
			}
		}
		if visible[index] {
			p.empty = false
		}
	}
	return p
}

// BuildColumn returns a bounded NVWC response containing independently positioned
// section frames. Only the requested vertical window and touching neighbours decode.
func (e *Engine) BuildColumn(ctx context.Context, source *Source, key chunkKey) ([]byte, error) {
	pos := celler.ChunkPos{X: key.X, Z: key.Z}
	positions := [5]celler.ChunkPos{pos, {X: pos.X - 1, Z: pos.Z}, {X: pos.X + 1, Z: pos.Z}, {X: pos.X, Z: pos.Z - 1}, {X: pos.X, Z: pos.Z + 1}}
	columns := map[celler.ChunkPos]map[int32]*prepared{}
	var center *java.Chunk
	low, high := key.MinY, key.MaxY
	for ci, cp := range positions {
		raw, err := source.Read(ctx, cp)
		if errors.Is(err, fs.ErrNotExist) {
			if ci == 0 {
				return emptyPacket(), nil
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		data, err := raw.Decode()
		if err != nil {
			return nil, fmt.Errorf("chunk %v: %w", cp, err)
		}
		c, err := java.DecodeChunk(data)
		if err != nil {
			return nil, err
		}
		actual, err := c.Position()
		if err != nil {
			return nil, err
		}
		if actual != cp {
			return nil, fmt.Errorf("chunk position %v does not match %v", actual, cp)
		}
		if ci == 0 {
			center = c
		}
		column := map[int32]*prepared{}
		for y := max(int32(-128), low-1); y <= min(int32(127), high+1); y++ {
			if err = ctx.Err(); err != nil {
				return nil, err
			}
			s, err := c.Section(y)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("chunk %v section %d: %w", cp, y, err)
			}
			column[y] = e.prepare(s)
		}
		columns[cp] = column
	}
	entities := map[int32]map[[3]uint8]string{}
	var issues []RenderIssue
	reported := map[string]bool{}
	report := func(issue RenderIssue) {
		key := issue.Blockstate
		if key == "" {
			key = fmt.Sprintf("%s:%v", issue.Reason, issue.Position)
		}
		if !reported[key] {
			reported[key] = true
			issues = append(issues, issue)
		}
	}
	invalidEntityList := false
	invalidEntities := map[[3]int32]bool{}
	if center != nil {
		if value, exists := center.NBT.Value["block_entities"]; exists {
			compounds, ok := nbt.Compounds(value)
			if !ok {
				invalidEntityList = true
				report(RenderIssue{
					Position: [3]int32{pos.X * 16, 0, pos.Z * 16},
					Reason:   "invalid_block_entities",
					Detail:   fmt.Sprintf("block_entities is not a compound list (%T)", value),
				})
			}
			for _, entity := range compounds {
				// Only the supported actor currently consumes NBT. Keep unrelated entities
				// out of the rendering payload and avoid formatting their inventories.
				if entity["id"] != "minecraft:decorated_pot" {
					continue
				}
				x, xOK := entity["x"].(int32)
				y, yOK := entity["y"].(int32)
				z, zOK := entity["z"].(int32)
				if !xOK || !yOK || !zOK || x>>4 != pos.X || z>>4 != pos.Z {
					report(RenderIssue{
						Position: [3]int32{pos.X * 16, 0, pos.Z * 16},
						Reason:   "invalid_block_entity",
						Detail:   "decorated_pot requires Int x/y/z inside its chunk",
					})
					continue
				}
				if y>>4 < low || y>>4 > high {
					continue
				}
				text, err := nbt.EncodeToSNBT(entity, nbt.OptEncodeToSNBT{})
				if err != nil {
					invalidEntities[[3]int32{x, y, z}] = true
					report(RenderIssue{
						Position: [3]int32{pos.X * 16, 0, pos.Z * 16},
						Reason:   "invalid_block_entity",
						Detail:   fmt.Sprintf("decorated_pot at %d,%d,%d: %v", x, y, z, err),
					})
					continue
				}
				if entities[y>>4] == nil {
					entities[y>>4] = map[[3]uint8]string{}
				}
				entities[y>>4][[3]uint8{uint8(x & 15), uint8(y & 15), uint8(z & 15)}] = text
			}
		}
	}
	out := bytes.NewBuffer(emptyPacket())
	count, unknown := uint32(0), uint32(0)
	for y := low; y <= high; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		section := columns[pos][y]
		if section == nil || section.empty {
			continue
		}
		unknown += uint32(section.unknown)
		for _, issue := range section.issues {
			issue.Position[0] += pos.X * 16
			issue.Position[2] += pos.Z * 16
			report(issue)
		}
		var neighbours [6][][]uint32
		adjacent := [6]struct {
			p celler.ChunkPos
			y int32
		}{{positions[1], y}, {positions[2], y}, {pos, y - 1}, {pos, y + 1}, {positions[3], y}, {positions[4], y}}
		for side, a := range adjacent {
			if s := columns[a.p][a.y]; s != nil {
				neighbours[side] = s.layers
			}
		}
		// The mesher uses unmodified native shapes for neighbour coverage. Actors
		// replace their own visible static shape after the section arrays are copied.
		layers := section.layers
		var actors []blockactor.Actor
		var effects []effector.Message
		for i := 0; i < 4096; i++ {
			lp := layer_storage.At(i>>8, i&15, i>>4&15)
			block := section.blocks.Get(lp)
			name := block.NameWithNameSpace()
			position := [3]uint8{lp.X, lp.Y, lp.Z}
			if name == "minecraft:enchanting_table" {
				effects = append(effects, effector.Message{Pos: position, Effect: "enchanting_table_book"})
			}
			if name != "minecraft:decorated_pot" || e.actor == nil {
				continue
			}
			// Missing resource states use the same visible marker as other blocks.
			if section.layers[0][i] == e.missing {
				continue
			}
			worldPosition := [3]int32{pos.X*16 + int32(lp.X), y*16 + int32(lp.Y), pos.Z*16 + int32(lp.Z)}
			// Entity data is optional decoration. Keep the static model if it
			// cannot be read, or if building the replacement appearance fails.
			if invalidEntityList || invalidEntities[worldPosition] {
				continue
			}
			built, err := e.actor.Build([]blockactor.Input{{Pos: position, State: block, SNBT: entities[y][position], Waterlogged: len(layers) > 1 && layers[1][i] != e.air}})
			if err != nil {
				report(RenderIssue{Position: worldPosition, Blockstate: string(block), Reason: "actor_error", Detail: err.Error()})
				continue
			}
			if len(actors) == 0 {
				layers = append([][]uint32(nil), section.layers...)
				layers[0] = append([]uint32(nil), section.layers[0]...)
			}
			actors = append(actors, built...)
			layers[0][i] = e.air
		}
		mesh, err := e.mesher.MeshWithNeighbours(layers, neighbours, mesher.Options{})
		if err != nil {
			return nil, err
		}
		if len(mesh.Mesh.Quads) == 0 && len(actors) == 0 && len(effects) == 0 {
			continue
		}
		var frame bytes.Buffer
		if err = (merger.Frame{Mesh: mesh.Mesh, Actors: actors, Effects: effects}).Marshal(&frame); err != nil {
			return nil, err
		}
		if out.Len()+frame.Len()+12 > maxColumnBytes {
			return nil, fmt.Errorf("column mesh exceeds 24 MiB; reduce view distance")
		}
		binary.Write(out, binary.LittleEndian, y)
		binary.Write(out, binary.LittleEndian, uint32(frame.Len()))
		binary.Write(out, binary.LittleEndian, uint32(section.unknown))
		out.Write(frame.Bytes())
		count++
	}
	var diagnostics []byte
	if len(issues) > 0 {
		var err error
		diagnostics, err = json.Marshal(issues)
		if err != nil {
			return nil, err
		}
	}
	if out.Len()+len(diagnostics) > maxColumnBytes {
		return nil, fmt.Errorf("column diagnostics exceed 24 MiB; reduce view distance")
	}
	out.Write(diagnostics)
	data := out.Bytes()
	binary.LittleEndian.PutUint32(data[16:], uint32(len(diagnostics)))
	binary.LittleEndian.PutUint32(data[8:], count)
	binary.LittleEndian.PutUint32(data[12:], unknown)
	return data, nil
}
func emptyPacket() []byte {
	data := make([]byte, 20)
	copy(data, "NVWC")
	binary.LittleEndian.PutUint32(data[4:], 3)
	return data
}
