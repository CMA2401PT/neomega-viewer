package main

import "embed"

// Regenerate the independent frontend and copy the matching texture resources.
//go:generate node bundle.mjs

// bundle includes the built frontend, atlas metadata, textures and notices.
// Geometry tables are embedded by neomega-blocks/presets/java/v1/geometry.
//
//go:embed bundle
var bundle embed.FS
