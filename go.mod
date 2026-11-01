module neomega-viewer

go 1.25.5

require (
	celler v0.0.0
	neomega-blocks v0.0.0
	neomega-converter v0.0.0
)

require (
	neomega-nbt v0.0.0
	neomega-packfs v0.0.0
)

require github.com/pierrec/lz4/v4 v4.1.22 // indirect

replace neomega-blocks => ../neomega-blocks

replace neomega-nbt => ../neomega-nbt

replace celler => ../neomega-celler

replace neomega-converter => ../neomega-converter

replace neomega-packfs => ../neomega-packfs
