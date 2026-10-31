package decoratedpot_test

import (
	"math"
	"reflect"
	"strings"
	"testing"

	blockgeometry "neomega-blocks/geometry"
	"neomega-blocks/geometry/material"
	"neomega-blocks/ir"
	resources "neomega-blocks/presets/java/v1/geometry"
	"neomega-viewer/blockactor"
	"neomega-viewer/blockactor/decoratedpot"
	"neomega-viewer/mesher"
	"neomega-viewer/mesher/geometry"
)

const listNBT = `{sherds:["minecraft:angler_pottery_sherd","minecraft:archer_pottery_sherd","minecraft:arms_up_pottery_sherd","minecraft:prize_pottery_sherd"]}`
const compoundNBT = `{sherds:{back:{id:"minecraft:angler_pottery_sherd"},left:{id:"minecraft:archer_pottery_sherd"},right:{id:"minecraft:arms_up_pottery_sherd"},front:{id:"minecraft:prize_pottery_sherd"}}}`

func TestDecoratedPot(t *testing.T) {
	file, err := resources.Files.Open("geometry.nvtables")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	tables, err := blockgeometry.DecodeTables[uint32](file)
	if err != nil {
		t.Fatal(err)
	}
	surface := func(name string) (material.Surface, error) {
		return material.Surface{Texture: name, Region: [4]float32{.25, .5, .125, .125}, Mode: material.Opaque, Opacity: 1, Frames: 1}, nil
	}
	handler := decoratedpot.New(tables, surface)
	actors := blockactor.New(handler)
	for _, test := range []struct {
		facing string
		// 顺序为 back、left、right、front，直接列出世界法向作为朝向契约。
		normals [4]geometry.Vec3
	}{
		{"north", [4]geometry.Vec3{{0, 0, -1}, {-1, 0, 0}, {1, 0, 0}, {0, 0, 1}}},
		{"east", [4]geometry.Vec3{{1, 0, 0}, {0, 0, -1}, {0, 0, 1}, {-1, 0, 0}}},
		{"south", [4]geometry.Vec3{{0, 0, 1}, {1, 0, 0}, {-1, 0, 0}, {0, 0, -1}}},
		{"west", [4]geometry.Vec3{{-1, 0, 0}, {0, 0, 1}, {0, 0, -1}, {1, 0, 0}}},
	} {
		t.Run(test.facing, func(t *testing.T) {
			state := ir.IRBlock(`minecraft:decorated_pot ["facing"="` + test.facing + `","cracked"="true"]`).Tidy()
			input := blockactor.Input{Pos: [3]uint8{4, 3, 7}, State: state, SNBT: listNBT}
			mesh, err := handler.Build(input)
			if err != nil {
				t.Fatal(err)
			}
			input.SNBT = compoundNBT
			compound, err := handler.Build(input)
			if err != nil || !reflect.DeepEqual(mesh, compound) {
				t.Fatalf("NBT representations differ: %v", err)
			}
			static, err := mesher.NewMesher(tables).MeshBlock(state.Hash())
			if err != nil {
				t.Fatal(err)
			}
			patterns := []string{"angler", "archer", "arms_up", "prize"}
			seen := [4]int{}
			for i, quad := range mesh.Quads {
				if !strings.HasSuffix(quad.Surface.Texture, "_pottery_pattern") {
					if quad != static.Quads[i] {
						t.Fatal("base geometry or material changed")
					}
					continue
				}
				for side, pattern := range patterns {
					if !strings.HasSuffix(quad.Surface.Texture, "/"+pattern+"_pottery_pattern") {
						continue
					}
					seen[side]++
					for axis := range quad.Normal {
						if math.Abs(quad.Normal[axis]-test.normals[side][axis]) > 1e-9 {
							t.Fatalf("%s normal=%v, want %v", pattern, quad.Normal, test.normals[side])
						}
					}
					if quad.UV != ([4]geometry.Vec2{{1.0 / 16, 0}, {1.0 / 16, 1}, {15.0 / 16, 1}, {15.0 / 16, 0}}) || quad.Surface.Region != ([4]float32{.25, .5, .125, .125}) {
						t.Fatal("pattern UV or injected atlas region differs")
					}
					if quad.Positions != static.Quads[i].Positions {
						t.Fatal("side geometry changed")
					}
				}
			}
			if seen != ([4]int{1, 1, 1, 1}) {
				t.Fatalf("side counts=%v", seen)
			}
			placed, err := actors.Build([]blockactor.Input{input})
			if err != nil {
				t.Fatal(err)
			}
			bounds := placed[0].Mesh.Buffers().Bounds
			if bounds.Min != (geometry.Vec3{4 + 1.0/16, 3, 7 + 1.0/16}) || bounds.Max != (geometry.Vec3{4 + 15.0/16, 4.25, 7 + 15.0/16}) || placed[0].Pos != input.Pos {
				t.Fatalf("translation or overheight lost: %+v", bounds)
			}
		})
	}
	for _, snbt := range []string{"", "{}", "{broken", `{sherds:[]}`, `{sherds:["minecraft:brick","","minecraft:brick","minecraft:brick"]}`, `{sherds:{front:{}}}`} {
		mesh, err := handler.Build(blockactor.Input{
			State: ir.IRBlock(`minecraft:decorated_pot ["facing"="north","cracked"="false","waterlogged"="true"]`).Tidy(),
			SNBT:  snbt,
		})
		if err != nil {
			t.Fatal(err)
		}
		sides := 0
		for _, quad := range mesh.Quads {
			if strings.HasSuffix(quad.Surface.Texture, "/decorated_pot_side") {
				sides++
			}
		}
		if sides != 4 {
			t.Fatalf("default sides=%d for %s", sides, snbt)
		}
	}
}
