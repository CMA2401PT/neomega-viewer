package bake

import (
	"math"

	"neomega-viewer/mesher/geometry"
)

func add(a, b geometry.Vec3) geometry.Vec3 {
	return geometry.Vec3{a[0] + b[0], a[1] + b[1], a[2] + b[2]}
}
func sub(a, b geometry.Vec3) geometry.Vec3 {
	return geometry.Vec3{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
}
func dot(a, b geometry.Vec3) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func cross(a, b geometry.Vec3) geometry.Vec3 {
	return geometry.Vec3{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func unit(v geometry.Vec3) geometry.Vec3 {
	length := math.Sqrt(dot(v, v))
	if length == 0 {
		return geometry.Vec3{}
	}
	return geometry.Vec3{v[0] / length, v[1] / length, v[2] / length}
}
func rotate(v geometry.Vec3, axis string, degrees float64) geometry.Vec3 {
	a := degrees * math.Pi / 180
	c, s := math.Cos(a), math.Sin(a)
	switch axis {
	case "x":
		return geometry.Vec3{v[0], c*v[1] - s*v[2], s*v[1] + c*v[2]}
	case "y":
		return geometry.Vec3{c*v[0] + s*v[2], v[1], -s*v[0] + c*v[2]}
	case "z":
		return geometry.Vec3{c*v[0] - s*v[1], s*v[0] + c*v[1], v[2]}
	}
	return v
}

var opposite = map[string]string{"north": "south", "south": "north", "east": "west", "west": "east", "up": "down", "down": "up"}
var faceAxis = map[string]int{"north": 2, "south": 2, "east": 0, "west": 0, "up": 1, "down": 1}

var directions = []string{"down", "up", "north", "south", "west", "east"}
var normals = map[string]geometry.Vec3{
	"down": {0, -1, 0}, "up": {0, 1, 0}, "north": {0, 0, -1}, "south": {0, 0, 1}, "west": {-1, 0, 0}, "east": {1, 0, 0},
}

// Corners run counterclockwise from the outside and correspond to texture
// top-left, bottom-left, bottom-right, top-right.
func corners(face string, lo, hi geometry.Vec3) [4]geometry.Vec3 {
	x, y, z, X, Y, Z := lo[0], lo[1], lo[2], hi[0], hi[1], hi[2]
	switch face {
	case "down":
		return [4]geometry.Vec3{{x, y, Z}, {x, y, z}, {X, y, z}, {X, y, Z}}
	case "up":
		return [4]geometry.Vec3{{x, Y, z}, {x, Y, Z}, {X, Y, Z}, {X, Y, z}}
	case "north":
		return [4]geometry.Vec3{{X, Y, z}, {X, y, z}, {x, y, z}, {x, Y, z}}
	case "south":
		return [4]geometry.Vec3{{x, Y, Z}, {x, y, Z}, {X, y, Z}, {X, Y, Z}}
	case "west":
		return [4]geometry.Vec3{{x, Y, z}, {x, y, z}, {x, y, Z}, {x, Y, Z}}
	case "east":
		return [4]geometry.Vec3{{X, Y, Z}, {X, y, Z}, {X, y, z}, {X, Y, z}}
	}
	return [4]geometry.Vec3{}
}

func direction(v geometry.Vec3) string {
	best, name := -2.0, ""
	for _, side := range directions {
		if score := dot(v, normals[side]); score > best {
			best, name = score, side
		}
	}
	return name
}
