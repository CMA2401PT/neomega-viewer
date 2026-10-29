// Package material separates surface sampling from geometry and world occlusion.
package material

import "neomega-assets/asset"

type Mode string

const (
	Opaque Mode = "opaque"
	Cutout Mode = "cutout"
	Blend  Mode = "blend"
)

type Surface struct {
	Texture asset.ID `json:"texture"`
	// Region is [u,v,width,height], normalized against the atlas, top-origin.
	Region      [4]float32 `json:"region"`
	Mode        Mode       `json:"mode"`
	DoubleSided bool       `json:"doubleSided"`
	Unlit       bool       `json:"unlit"`
	Opacity     float32    `json:"opacity"`
}
