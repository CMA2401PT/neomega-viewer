package meshing

import (
	"math/rand"
	"reflect"
	"slices"
	"sync"
	"testing"

	blockgeometry "neomega-blocks/geometry"
	"neomega-viewer/mesher/geometry"
)

func TestClipBuffersMatchSharedSubtract(t *testing.T) {
	unit := box{Max: [2]float64{1, 1}}
	cases := [][]box{
		{unit},
		{{Min: [2]float64{1, 0}, Max: [2]float64{2, 1}}},
		{{Min: [2]float64{epsilon / 2, epsilon / 2}, Max: [2]float64{1 - epsilon/2, 1 - epsilon/2}}},
		{{Min: [2]float64{1 - epsilon/2, 0}, Max: [2]float64{2, 1}}},
	}
	// Produce more fragments than the inline capacity, then reuse the buffers for
	// other faces. Compare values and order with the shared geometry operation.
	var stripes []box
	for i := 1; i < 20; i++ {
		x := float64(i) / 20
		stripes = append(stripes, box{Min: [2]float64{x, 0}, Max: [2]float64{x + .01, 1}})
	}
	cases = append(cases, stripes)
	rng := rand.New(rand.NewSource(17))
	for range 100 {
		var covers []box
		for range 12 {
			x, y := rng.Float64()*1.5-.25, rng.Float64()*1.5-.25
			covers = append(covers, box{Min: [2]float64{x, y}, Max: [2]float64{x + rng.Float64(), y + rng.Float64()}})
		}
		cases = append(cases, covers)
	}
	var clip clipBuffers
	for i, covers := range cases {
		clip.reset(unit)
		want := []box{unit}
		for j, cover := range covers {
			var next []box
			for _, piece := range want {
				next = append(next, blockgeometry.Subtract(piece, cover)...)
			}
			want = next
			clip.subtract(cover)
			if !slices.Equal(clip.remaining, want) {
				t.Fatalf("case %d cover %d: got %v, want %v", i, j, clip.remaining, want)
			}
		}
	}
}

func TestBuildInspectionLifetimeAndConcurrentInputs(t *testing.T) {
	source := &geometry.Shape{Quads: []geometry.Quad{face(0, 1, 1, [2]float64{}, [2]float64{1, 1}, opaque)}}
	cover := &geometry.Shape{Quads: []geometry.Quad{face(0, -1, 0, [2]float64{}, [2]float64{.5, 1}, opaque)}}
	source.Boundary = blockgeometry.BoundaryCoverage(source.Quads)
	cover.Boundary = blockgeometry.BoundaryCoverage(cover.Quads)
	var cells Cells
	cells[0] = Cell{{Shape: source}}
	cells[256] = Cell{{Shape: cover}}
	options := Options{DisableGreedy: true}
	before, beforeStats := Build(&cells, options)
	if len(before.Quads) != 1 || beforeStats.ClippedQuads != 1 {
		t.Fatalf("expected one partially clipped source face: %+v", beforeStats)
	}
	// Inputs are immutable during a build, but may change between calls. Cached
	// affine UV inspection must not survive the call that owns it.
	for i := range source.Quads[0].UV {
		source.Quads[0].UV[i][0] += 3
	}
	want := geometry.Mesh{Quads: slices.Clone(before.Quads)}
	for i := range want.Quads[0].UV {
		want.Quads[0].UV[i][0] += 3
	}
	original := source.Quads[0]
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			got, stats := Build(&cells, options)
			if !reflect.DeepEqual(got, want) || stats != beforeStats {
				t.Errorf("independent Build changed geometry or stats: %+v", stats)
			}
		})
	}
	workers.Wait()
	if source.Quads[0] != original {
		t.Fatal("Build changed shared shape")
	}
}

func BenchmarkSharedShapeCulling(b *testing.B) {
	cube := &geometry.Shape{Quads: cuboid(1, opaque).Quads}
	slab := &geometry.Shape{Quads: cuboid(.5, opaque).Quads}
	cube.Boundary = blockgeometry.BoundaryCoverage(cube.Quads)
	slab.Boundary = blockgeometry.BoundaryCoverage(slab.Quads)
	for _, name := range []string{"solid", "partial"} {
		b.Run(name, func(b *testing.B) {
			var cells Cells
			for i := range cells {
				shape := cube
				if name == "partial" && i%3 == 0 {
					shape = slab
				}
				cells[i] = Cell{{Shape: shape}}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				Build(&cells, Options{})
			}
		})
	}
}
