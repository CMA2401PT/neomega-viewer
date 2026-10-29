package resources

import (
	"strings"

	"neomega-viewer/blockstate"
)

// Neighbor describes the static facts needed to select connection parts. Solid
// means actual opaque geometry covers the face toward the current block.
type Neighbor struct {
	State blockstate.State
	Solid bool
}

// ConnectionPolicy can replace the default preview rules for another palette
// or an application with its own static attachment semantics.
type ConnectionPolicy func(blockstate.State, map[string]Neighbor) map[string]string

// Connections supplies static fence/pane links that Bedrock IDs do not encode.
// Shape clipping itself belongs to the mesher, not to these family rules.
func Connections(state blockstate.State, neighbors map[string]Neighbor) map[string]string {
	name := state.Name.Path()
	fence := strings.HasSuffix(name, "_fence")
	pane := strings.HasSuffix(name, "_glass_pane") || name == "glass_pane" || name == "iron_bars"
	if !fence && !pane {
		return nil
	}
	result := map[string]string{}
	for _, side := range []string{"north", "east", "south", "west"} {
		neighbor := neighbors[side]
		other := neighbor.State.Name.Path()
		attached := neighbor.Solid
		if fence {
			sameKind := strings.HasSuffix(other, "_fence") && (name == "nether_brick_fence") == (other == "nether_brick_fence")
			gate := strings.HasSuffix(other, "_fence_gate")
			facing := neighbor.State.Properties["facing"]
			acrossGate := ((facing == "north" || facing == "south") && (side == "east" || side == "west")) || ((facing == "east" || facing == "west") && (side == "north" || side == "south"))
			attached = attached || sameKind || (gate && acrossGate)
		} else {
			attached = attached || other == "iron_bars" || other == "glass_pane" || strings.HasSuffix(other, "_glass_pane") || other == "glass" || other == "tinted_glass" || strings.HasSuffix(other, "_stained_glass") || strings.HasSuffix(other, "_wall")
		}
		value := "false"
		if attached {
			value = "true"
		}
		if state.Properties[side] != value {
			result[side] = value
		}
	}
	return result
}
