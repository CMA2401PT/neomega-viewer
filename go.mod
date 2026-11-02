module neomega-viewer

go 1.25.5

require (
	celler v0.0.0
	neomega-blocks v0.0.0
)

require (
	neomega-nbt v0.0.0
	neomega-packfs v0.0.0
)

require (
	github.com/df-mc/goleveldb v1.1.9 // indirect
	github.com/golang/snappy v0.0.1 // indirect
	github.com/klauspost/compress v1.13.6 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
)

require (
	neomega-converter v0.0.0
	neomega-converter-core v0.0.0
	neomega-viewer-core v0.0.0
)

replace celler => ../neomega-celler

replace neomega-blocks => ../neomega-blocks

replace neomega-converter => ../neomega-converter

replace neomega-converter-core => ../neomega-converter-core

replace neomega-nbt => ../neomega-nbt

replace neomega-packfs => ../neomega-packfs

replace neomega-viewer-core => ../neomega-viewer-core
