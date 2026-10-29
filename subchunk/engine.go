// Package subchunk renders one isolated 16x16x16 palette layer. It has no world
// loader or worker and shares only block parsing with the single-block entry.
package subchunk

import (
	"encoding/json"
	"fmt"
	"strings"

	"celler/layer_storage"
	"neomega-assets/asset"
	"neomega-viewer/blockmesh"
	"neomega-viewer/blockstate"
	"neomega-viewer/geometry"
	"neomega-viewer/meshing"
	"neomega-viewer/model"
)

type Options struct {
	// VaryModels derives weighted model choices from each cell position. With
	// false, one seed is shared, which also makes merging easier to inspect.
	VaryModels bool
	Seed       uint64
	Meshing    meshing.Options
}

type Stats struct {
	meshing.Stats
	// Blocks counts cells with local geometry before neighbor culling.
	Blocks      int `json:"blocks"`
	BakedShapes int `json:"bakedShapes"`
}

type Result struct {
	Mesh  geometry.Mesh
	Stats Stats
}
type Engine struct {
	Blocks *blockmesh.Resolver
}

func New(blocks *blockmesh.Resolver) *Engine {
	return &Engine{Blocks: blocks}
}

type shapeKey struct {
	ID         uint32
	Seed       uint64
	Properties string
}
type cachedShape struct {
	Block *blockmesh.Block
}

func hasAlternatives(state asset.BlockState) bool {
	for _, choice := range state.Variants {
		if len(choice.Options) > 1 {
			return true
		}
	}
	for _, part := range state.Multipart {
		if len(part.Apply.Options) > 1 {
			return true
		}
	}
	return false
}

// Build snapshots all 4096 logical cells. Palette's default can be stone or any
// other ID; IterNonDefault alone cannot describe a filled section.
func (engine *Engine) Build(layer *layer_storage.Palette[uint32], options Options) (*Result, error) {
	var ids [Volume]uint32
	for i := range ids {
		ids[i] = layer.Get(position(i))
	}
	shapes := map[shapeKey]*cachedShape{}
	bindings := map[uint32]blockstate.Binding{}
	sensitive := map[uint32]bool{}
	resolve := func(id uint32, seed uint64, properties map[string]string) (*cachedShape, error) {
		propertiesKey := ""
		if len(properties) > 0 {
			encoded, _ := json.Marshal(properties)
			propertiesKey = string(encoded)
		}
		key := shapeKey{ID: id, Seed: seed, Properties: propertiesKey}
		if hit := shapes[key]; hit != nil {
			return hit, nil
		}
		binding, known := bindings[id]
		if !known {
			var err error
			binding, err = engine.Blocks.Binding(id)
			if err != nil {
				return nil, err
			}
			bindings[id] = binding
		}
		block, err := engine.Blocks.Bake(binding, blockmesh.Options{Seed: seed, Properties: properties})
		if err != nil {
			return nil, err
		}
		shape := &cachedShape{Block: block}
		shapes[key] = shape
		return shape, nil
	}
	// Cache unmodified local shapes; only weighted choices need a seed per cell.
	var bases [Volume]*cachedShape
	var seeds [Volume]uint64
	for i, id := range ids {
		if _, known := sensitive[id]; !known {
			base, err := resolve(id, 0, nil)
			if err != nil {
				p := position(i)
				return nil, fmt.Errorf("cell (%d,%d,%d), ID %d: %w", p.X, p.Y, p.Z, id, err)
			}
			definition, _ := engine.Blocks.Assets.BlockState(base.Block.State.Name)
			sensitive[id] = hasAlternatives(definition)
		}
		seeds[i] = options.Seed
		if options.VaryModels && sensitive[id] {
			seeds[i] = model.Mix(options.Seed + uint64(i))
		}
		base, err := resolve(id, seeds[i], nil)
		if err != nil {
			return nil, err
		}
		bases[i] = base
	}
	var cells meshing.Cells
	blocks := 0
	for i, base := range bases {
		p := position(i)
		state := base.Block.State
		properties := map[string]string{}
		if base.Block.Renderer.Kind == asset.RendererFluid && p.Y < 15 {
			above := bases[index(int(p.X), int(p.Y)+1, int(p.Z))]
			if above.Block.Renderer.Kind == asset.RendererFluid && above.Block.Renderer.Family == base.Block.Renderer.Family {
				properties["level"] = "8"
			}
		}
		shape, err := resolve(ids[i], seeds[i], properties)
		if err != nil {
			return nil, err
		}
		mesh := shape.Block.Mesh
		group := ""
		if strings.Contains(state.Name.Path(), "glass") {
			group = "glass:" + string(state.Name)
		}
		if base.Block.Renderer.Kind == asset.RendererFluid {
			group = "fluid:" + base.Block.Renderer.Family
			// FluidPreview deliberately has no neighbor culling metadata. Give
			// its actual unit-boundary faces that meaning only in this entry.
			mesh.Quads = append([]geometry.Quad(nil), mesh.Quads...)
			for q := range mesh.Quads {
				mesh.Quads[q].CullFace = fluidBoundary(mesh.Quads[q])
			}
		}
		cells[i] = meshing.Cell{Mesh: mesh, CullGroup: group}
		if len(mesh.Quads) > 0 {
			blocks++
		}
	}
	mesh, stats := meshing.Build(&cells, options.Meshing)
	return &Result{Mesh: mesh, Stats: Stats{Stats: stats, Blocks: blocks, BakedShapes: len(shapes)}}, nil
}

func fluidBoundary(quad geometry.Quad) string {
	for axis := 0; axis < 3; axis++ {
		n := quad.Normal[axis]
		if n != 1 && n != -1 {
			continue
		}
		edge := float64(0)
		if n > 0 {
			edge = 1
		}
		boundary := true
		for _, p := range quad.Positions {
			boundary = boundary && p[axis] == edge
		}
		if boundary {
			if n > 0 {
				return []string{"east", "up", "south"}[axis]
			}
			return []string{"west", "down", "north"}[axis]
		}
	}
	return ""
}
