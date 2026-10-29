package blockmesh

import (
	"fmt"
	"maps"
	"slices"

	"neomega-assets/asset"
	"neomega-viewer/blockstate"
	"neomega-viewer/geometry"
	"neomega-viewer/meshing"
	"neomega-viewer/model"
	"neomega-viewer/resources"
)

type ModelID uint32

type SkippedState struct {
	Binding blockstate.Binding
	Err     error
}

type Tables struct {
	// 表和查询返回的切片共享且只读，供多个渲染入口复用。
	Index   map[uint32][]ModelID
	Shapes  []geometry.Shape
	Skipped []SkippedState
}

func (engine *Resolver) Tables() (*Tables, error) {
	// 每套资源和材质策略各建一次，避免不同 Open 配置串用几何。
	return engine.tables()
}

func (tables *Tables) ModelIDs(javaHash uint32) ([]ModelID, error) {
	ids, ok := tables.Index[javaHash]
	if !ok {
		return nil, fmt.Errorf("no static geometry for Java block hash %d", javaHash)
	}
	return ids, nil
}

func (tables *Tables) Shape(id ModelID) (*geometry.Shape, error) {
	if uint64(id) >= uint64(len(tables.Shapes)) {
		return nil, fmt.Errorf("unknown model ID %d", id)
	}
	return &tables.Shapes[id], nil
}

type shapeBucket struct {
	// 面数和首面只缩小查找范围，完整四边形序列相等才共享 ID。
	// Bake 受状态的材质与颜色影响，仅按模型路径和旋转会误合并。
	Count int
	First geometry.Quad
}

func (tables *Tables) intern(mesh geometry.Mesh, buckets map[shapeBucket][]ModelID) ModelID {
	key := shapeBucket{Count: len(mesh.Quads)}
	if len(mesh.Quads) != 0 {
		key.First = mesh.Quads[0]
	}
	for _, id := range buckets[key] {
		if slices.Equal(tables.Shapes[id].Quads, mesh.Quads) {
			return id
		}
	}
	id := ModelID(len(tables.Shapes))
	tables.Shapes = append(tables.Shapes, geometry.Shape{
		Quads:    mesh.Quads,
		Boundary: meshing.BoundaryCoverage(mesh),
	})
	buckets[key] = append(buckets[key], id)
	return id
}

func (engine *Resolver) buildTables() (*Tables, error) {
	states, err := blockstate.Java()
	if err != nil {
		return nil, err
	}
	tables := &Tables{Index: make(map[uint32][]ModelID, len(states.ByID))}
	buckets := map[shapeBucket][]ModelID{}
	// 哈希不稠密，用 map 索引；排序只为让同一套资源的编号可复现。
	for _, javaHash := range slices.Sorted(maps.Keys(states.ByID)) {
		binding := states.ByID[javaHash]
		meshes, err := engine.bakeParts(binding.Java)
		if err != nil {
			tables.Skipped = append(tables.Skipped, SkippedState{Binding: binding, Err: err})
			continue
		}
		// 整个状态成功后再编号，失败状态不会留下部分索引或无引用的 Shape。
		ids := make([]ModelID, 0, len(meshes))
		for _, mesh := range meshes {
			ids = append(ids, tables.intern(mesh, buckets))
		}
		tables.Index[javaHash] = ids
	}
	return tables, nil
}

func (engine *Resolver) bakeParts(state blockstate.State) ([]geometry.Mesh, error) {
	profile, ok := engine.Assets.Renderers.Blocks[state.Name]
	if !ok {
		return nil, fmt.Errorf("no renderer entry for %s", state.Name)
	}
	switch profile.Kind {
	case asset.RendererInvisible:
		return nil, nil
	case asset.RendererDynamic:
		return nil, fmt.Errorf("%s has no fixed static shape", state.Name)
	case asset.RendererFluid:
		mesh, err := geometry.Bake(
			resources.FluidPreview(state, profile.Family),
			asset.Apply{},
			state,
			engine.materials,
		)
		return []geometry.Mesh{mesh}, err
	}
	definition, ok := engine.Assets.BlockState(state.Name)
	if !ok {
		return nil, fmt.Errorf("blockstate %s missing", state.Name)
	}
	if _, needed := model.PropertyValues(definition, nil)["waterlogged"]; needed {
		state = resources.PreviewProperties(state)
	}
	applications, err := model.SelectFirst(definition, state.Properties)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", state.Name, err)
	}
	meshes := make([]geometry.Mesh, 0, len(applications))
	for _, application := range applications {
		id, err := asset.ParseID(string(application.Model))
		if err != nil {
			return nil, err
		}
		resolved, err := engine.models.Resolve(id)
		if err != nil {
			return nil, err
		}
		mesh, err := geometry.Bake(
			resolved,
			application,
			state,
			engine.materials,
		)
		if err != nil {
			return nil, fmt.Errorf("model %s: %w", id, err)
		}
		meshes = append(meshes, mesh)
	}
	return meshes, nil
}
