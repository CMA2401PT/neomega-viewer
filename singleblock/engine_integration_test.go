package singleblock_test

import (
	"maps"
	"math"
	"os"
	"slices"
	"sync"
	"testing"

	"neomega-assets/asset"
	"neomega-viewer/catalog"
	"neomega-viewer/geometry"
	"neomega-viewer/material"
	"neomega-viewer/singleblock"
)

func openReal(t *testing.T) (*singleblock.Engine, *catalog.Catalog) {
	t.Helper()
	dir := os.Getenv("NEOMEGA_ASSETS_DIR")
	if dir == "" {
		t.Skip("set NEOMEGA_ASSETS_DIR to run real resource/palette checks")
	}
	engine, err := singleblock.Open(singleblock.OpenOptions{AssetsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := catalog.Java()
	if err != nil {
		t.Fatal(err)
	}
	return engine, entries
}

func stateID(t *testing.T, entries *catalog.Catalog, name string, properties map[string]string) uint32 {
	t.Helper()
	for _, binding := range entries.ByName["minecraft:"+name] {
		if maps.Equal(binding.Java.Properties, properties) {
			return binding.ID
		}
	}
	t.Fatalf("state not found: %s %v", name, properties)
	return 0
}

func TestStaticShapesAndSurfaceTypes(t *testing.T) {
	engine, entries := openReal(t)
	ids := map[string]uint32{}
	for id, binding := range entries.ByID {
		ids[string(binding.Java.Name)] = id
	}
	for _, name := range []string{"stone", "glass", "oak_slab", "oak_fence", "short_grass", "fire", "chest", "red_bed", "oak_wall_hanging_sign", "oak_leaves", "air", "water", "lava"} {
		t.Run(name, func(t *testing.T) {
			id, ok := ids["minecraft:"+name]
			if !ok {
				t.Fatal("example is absent from the real catalog")
			}
			if name == "oak_slab" {
				id = stateID(t, entries, name, map[string]string{"type": "bottom"})
			}
			block, err := engine.Build(id)
			if err != nil {
				t.Fatal(err)
			}
			if name == "air" {
				if len(block.Mesh.Quads) != 0 {
					t.Fatal("air has geometry")
				}
				return
			}
			if len(block.Mesh.Quads) == 0 {
				t.Fatal("visible static block has no geometry")
			}
			bounds := block.Mesh.Buffers().Bounds
			if name == "oak_slab" && math.Abs(bounds.Max[1]-bounds.Min[1]-0.5) > 1e-6 {
				t.Fatalf("slab lost its half height: %+v", bounds)
			}
			if name == "stone" {
				if _, exists := block.Binding.Java.Properties["waterlogged"]; exists {
					t.Fatal("solid stone exposed a fabricated waterlogged property")
				}
			}
			if name == "short_grass" {
				for _, quad := range block.Mesh.Quads {
					if quad.Surface.DoubleSided {
						t.Fatal("explicit front/back plant planes would overlap")
					}
				}
			}
			if name == "glass" && block.Mesh.Quads[0].Surface.Mode != material.Blend {
				t.Fatal("glass is not blended")
			}
			if name == "oak_leaves" && block.Mesh.Quads[0].Surface.Mode != material.Cutout {
				t.Fatal("leaf alpha holes became full transparency")
			}
			if name == "oak_fence" {
				isolated, err := engine.Build(stateID(t, entries, name, map[string]string{"north": "false", "east": "false", "south": "false", "west": "false"}))
				if err != nil {
					t.Fatal(err)
				}
				connected, err := engine.Build(stateID(t, entries, name, map[string]string{"north": "true", "east": "true", "south": "false", "west": "false"}))
				if err != nil {
					t.Fatal(err)
				}
				if len(connected.Mesh.Quads) <= len(isolated.Mesh.Quads) {
					t.Fatal("multipart connections were not added")
				}
			}
		})
	}
}

func TestEveryPaletteStateHasAStaticResultOrAnExplicitStaticBoundary(t *testing.T) {
	engine, entries := openReal(t)
	checked, unsupported := 0, 0
	for id, binding := range entries.ByID {
		block, err := engine.Build(id)
		profile := engine.Assets.Renderers.Blocks[binding.Java.Name]
		if profile.Kind == asset.RendererDynamic {
			if err == nil {
				t.Fatalf("runtime-only shape silently accepted: %+v", binding.Java)
			}
			unsupported++
			continue
		}
		if err != nil {
			t.Fatalf("%d %+v: %v", id, binding.Java, err)
		}
		for _, quad := range block.Mesh.Quads {
			for _, position := range quad.Positions {
				for _, value := range position {
					if math.IsNaN(value) || math.IsInf(value, 0) {
						t.Fatal("non-finite vertex")
					}
				}
			}
		}
		checked++
	}
	t.Logf("rendered %d palette states; %d have no fixed static shape", checked, unsupported)
	// The same immutable caches can serve simultaneous requests.
	var wg sync.WaitGroup
	for _, group := range entries.Groups[:8] {
		wg.Add(1)
		go func(id uint32) {
			defer wg.Done()
			if _, err := engine.Build(id); err != nil {
				t.Error(err)
			}
		}(group.ExampleID)
	}
	wg.Wait()
}

func TestBuildUsesSharedShapes(t *testing.T) {
	engine, entries := openReal(t)
	tables, err := engine.Tables()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"oak_slab", "oak_fence", "redstone_wire", "water", "air"} {
		for _, binding := range entries.ByName["minecraft:"+name] {
			block, err := engine.Build(binding.ID)
			if err != nil {
				t.Fatal(err)
			}
			ids, err := tables.ModelIDs(binding.ID)
			if err != nil {
				t.Fatal(err)
			}
			var want []geometry.Quad
			for _, id := range ids {
				shape, err := tables.Shape(id)
				if err != nil {
					t.Fatal(err)
				}
				want = append(want, shape.Quads...)
			}
			if !slices.Equal(block.Mesh.Quads, want) {
				t.Fatalf("%+v differs from shared shapes", binding)
			}
			if len(block.Mesh.Quads) != 0 {
				shape, _ := tables.Shape(ids[0])
				original := shape.Quads[0]
				block.Mesh.Quads[0].Color = [3]float32{-1, -1, -1}
				if shape.Quads[0] != original {
					t.Fatal("output mutation changed a shared shape")
				}
			}
		}
	}
}
