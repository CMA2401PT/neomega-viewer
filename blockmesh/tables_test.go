package blockmesh

import (
	"maps"
	"os"
	"slices"
	"testing"

	"neomega-assets/asset"
	"neomega-viewer/blockstate"
	"neomega-viewer/geometry"
)

func TestTablesWithJavaResources(t *testing.T) {
	dir := os.Getenv("NEOMEGA_ASSETS_DIR")
	if dir == "" {
		t.Skip("set NEOMEGA_ASSETS_DIR to run real resource checks")
	}
	engine, err := Open(OpenOptions{AssetsDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	tables, err := engine.Tables()
	if err != nil {
		t.Fatal(err)
	}
	again, err := engine.Tables()
	if err != nil || again != tables {
		t.Fatal("tables were not shared")
	}
	states, err := blockstate.Java()
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(name string, properties map[string]string, count int) []ModelID {
		t.Helper()
		for hash, binding := range states.ByID {
			if binding.Java.Name != asset.ID("minecraft:"+name) || !maps.Equal(binding.Java.Properties, properties) {
				continue
			}
			ids, err := tables.ModelIDs(hash)
			if err != nil || len(ids) != count {
				t.Fatalf("%s %v: IDs=%v, want %d models, error=%v", name, properties, ids, count, err)
			}
			for _, id := range ids {
				shape, err := tables.Shape(id)
				if err != nil || len(shape.Quads) == 0 {
					t.Fatalf("%s: empty or missing shape %d: %v", name, id, err)
				}
			}
			return ids
		}
		t.Fatalf("state not found: %s %v", name, properties)
		return nil
	}

	fence := map[string]string{
		"north": "false", "east": "false", "south": "false", "west": "false",
	}
	post := lookup("oak_fence", fence, 1)
	fence["north"], fence["east"] = "true", "true"
	connected := lookup("oak_fence", fence, 3)
	if post[0] != connected[0] {
		t.Fatal("fence states did not share the post")
	}
	if connected[1] == connected[2] {
		t.Fatal("differently rotated fence rails were merged")
	}
	fence["south"] = "true"
	if !slices.Equal(connected, lookup("oak_fence", fence, 4)[:3]) {
		t.Fatal("additional fence connection duplicated existing parts")
	}

	slab := map[string]string{"type": "bottom"}
	bottom := lookup("oak_slab", slab, 1)
	slab["type"] = "top"
	top := lookup("oak_slab", slab, 1)
	slab["type"] = "double"
	double := lookup("oak_slab", slab, 1)
	if bottom[0] == top[0] || bottom[0] == double[0] || top[0] == double[0] {
		t.Fatal("different slab shapes were merged")
	}
	shape, _ := tables.Shape(bottom[0])
	if len(shape.Boundary[3]) != 0 || len(shape.Boundary[2]) != 1 {
		t.Fatal("slab top inside the cell was counted as a boundary")
	}
	if len(shape.Boundary[4]) != 1 || shape.Boundary[4][0].Max != (geometry.Vec2{1, 0.5}) {
		t.Fatalf("slab partial coverage lost: %+v", shape.Boundary[4])
	}

	wire := map[string]string{"north": "none", "east": "none", "south": "none", "west": "none", "power": "0"}
	dot := lookup("redstone_wire", wire, 1)
	wire["north"], wire["east"] = "side", "side"
	corner := lookup("redstone_wire", wire, 3)
	if dot[0] != corner[0] {
		t.Fatal("redstone states did not share their dot")
	}
	wire["power"] = "15"
	powered := lookup("redstone_wire", wire, 3)
	if dot[0] == powered[0] {
		t.Fatal("existing baked tint differences were lost during deduplication")
	}

	stone := lookup("stone", nil, 1)
	shape, _ = tables.Shape(stone[0])
	if len(shape.Quads) != 6 {
		t.Fatalf("stone has %d quads", len(shape.Quads))
	}
	for side, rectangles := range shape.Boundary {
		if len(rectangles) != 1 || rectangles[0].Min != (geometry.Vec2{}) || rectangles[0].Max != (geometry.Vec2{1, 1}) {
			t.Fatalf("stone boundary %d: %+v", side, rectangles)
		}
	}
	lookup("air", nil, 0)
	lookup("water", map[string]string{"level": "0"}, 1)
	lookup("lava", map[string]string{"level": "8"}, 1)

	if len(tables.Index)+len(tables.Skipped) != len(states.ByID) {
		t.Fatal("some Java states were neither indexed nor reported")
	}
	if len(tables.Skipped) != 12 {
		t.Fatalf("unexpected skipped states: %+v", tables.Skipped)
	}
	for _, skipped := range tables.Skipped {
		if skipped.Binding.Java.Name != "minecraft:moving_piston" || skipped.Err == nil {
			t.Fatalf("unexpected skipped state: %+v", skipped)
		}
		if _, err := tables.ModelIDs(skipped.Binding.ID); err == nil {
			t.Fatal("skipped state has an index entry")
		}
	}
	references := 0
	used := map[ModelID]bool{}
	for _, ids := range tables.Index {
		references += len(ids)
		for _, id := range ids {
			used[id] = true
		}
	}
	if len(used) != len(tables.Shapes) || references <= len(tables.Shapes) {
		t.Fatal("geometry table has unused shapes or did not deduplicate")
	}
	t.Logf("Java states=%d; indexed=%d; shapes=%d; references=%d; skipped=%d (minecraft:moving_piston)",
		len(states.ByID), len(tables.Index), len(tables.Shapes), references, len(tables.Skipped))
}

func TestInternComparesAllBakedQuadFields(t *testing.T) {
	tables := &Tables{}
	buckets := map[shapeBucket][]ModelID{}
	original := geometry.Mesh{Quads: []geometry.Quad{{}, {Color: [3]float32{1, 1, 1}}}}
	first := tables.intern(original, buckets)
	copy := geometry.Mesh{Quads: slices.Clone(original.Quads)}
	if tables.intern(copy, buckets) != first {
		t.Fatal("identical geometry did not share an ID")
	}
	copy.Quads[1].Color = [3]float32{1, 0, 0}
	if tables.intern(copy, buckets) == first {
		t.Fatal("a difference after the first quad was ignored")
	}
}
