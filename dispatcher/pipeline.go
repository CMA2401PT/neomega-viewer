package dispatcher

import (
	"neomega-viewer/blockactor"
	"neomega-viewer/effector"
	"neomega-viewer/merger"
	"neomega-viewer/mesher"
	"neomega-viewer/splitlayer"
)

type Pipeline struct {
	// 组装结果只读共享，每次 Build 单独产生一个子区块快照。
	java    *Java
	mesher  *mesher.Mesher[uint32]
	effects *effector.Effector
	actors  *blockactor.Dispatcher
	fix     FixFunc
}

type Options struct {
	Mesh mesher.Options
	Fix  bool
}

type Result struct {
	Frame merger.Frame
	Stats mesher.Stats
}

func NewPipeline(
	tables *mesher.Tables[uint32],
	effects *effector.Effector,
	actors *blockactor.Dispatcher,
	fix FixFunc,
) *Pipeline {
	// 从已启用的处理器生成路由，认领范围和实际可调用的处理器保持一致。
	return &Pipeline{
		java:    NewJava(JavaRoutes(effects.BlockNames(), actors.BlockNames())),
		mesher:  mesher.NewMesher(tables),
		effects: effects,
		actors:  actors,
		fix:     fix,
	}
}

func (p *Pipeline) Build(blocks []splitlayer.JavaBlockHash, nbt map[[3]uint8]string, options Options) (*Result, error) {
	// 输入为 4096 个完整 Java hash，Frame 可直接 Marshal 成 merger 容器。
	var fix FixFunc
	if options.Fix {
		fix = p.fix
	}
	inputs := p.java.Dispatch(blocks, nbt, fix)
	static, err := p.mesher.MeshMultiLayerSubchunks(inputs.Layers, options.Mesh)
	if err != nil {
		return nil, err
	}
	actors, err := p.actors.Build(inputs.Actors)
	if err != nil {
		return nil, err
	}
	return &Result{
		Frame: merger.Frame{
			Mesh:    static.Mesh,
			Effects: p.effects.Build(inputs.Effects),
			Actors:  actors,
		},
		Stats: static.Stats,
	}, nil
}
