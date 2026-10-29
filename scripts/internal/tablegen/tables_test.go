package tablegen

import (
	"slices"
	"testing"

	"neomega-viewer/mesher"
	"neomega-viewer/mesher/geometry"
)

func TestInternComparesAllBakedQuadFields(t *testing.T) {
	tables := &mesher.Tables[uint32]{}
	buckets := map[shapeBucket][]mesher.ModelID{}
	original := geometry.Mesh{Quads: []geometry.Quad{{}, {Color: [3]float32{1, 1, 1}}}}
	first := intern(tables, original, 0, buckets)
	copy := geometry.Mesh{Quads: slices.Clone(original.Quads)}
	if intern(tables, copy, 0, buckets) != first {
		t.Fatal("identical geometry did not share an ID")
	}
	if intern(tables, copy, 1, buckets) == first {
		t.Fatal("different cull groups shared a shape")
	}
	copy.Quads[1].Color = [3]float32{1, 0, 0}
	if intern(tables, copy, 0, buckets) == first {
		t.Fatal("a difference after the first quad was ignored")
	}
}
