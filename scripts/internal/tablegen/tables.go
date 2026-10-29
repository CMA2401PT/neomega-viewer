package tablegen

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"neomega-assets/asset"
	"neomega-viewer/blockstate"
	"neomega-viewer/mesher"
	"neomega-viewer/mesher/geometry"
	"neomega-viewer/mesher/meshing"
	"neomega-viewer/scripts/internal/bake"
	"neomega-viewer/scripts/internal/model"
	"neomega-viewer/scripts/internal/resources"
)

type shapeBucket struct {
	// 面数和首面只缩小查找范围，完整四边形序列相等才共享 ID。
	// Bake 受状态的材质与颜色影响，仅按模型路径和旋转会误合并。
	Group uint32
	Count int
	First geometry.Quad
}

func intern(tables *mesher.Tables[uint32], mesh geometry.Mesh, group uint32, buckets map[shapeBucket][]mesher.ModelID) mesher.ModelID {
	key := shapeBucket{Group: group, Count: len(mesh.Quads)}
	if len(mesh.Quads) != 0 {
		key.First = mesh.Quads[0]
	}
	for _, id := range buckets[key] {
		if slices.Equal(tables.Shapes[id].Quads, mesh.Quads) {
			return id
		}
	}
	id := mesher.ModelID(len(tables.Shapes))
	tables.Shapes = append(tables.Shapes, geometry.Shape{
		Quads:     mesh.Quads,
		CullGroup: group,
		Boundary:  meshing.BoundaryCoverage(mesh),
	})
	buckets[key] = append(buckets[key], id)
	return id
}

type Result struct {
	*mesher.Tables[uint32]
	blockstate.Metadata
}

func (engine *Builder) Build() (*Result, error) {
	states, err := blockstate.Java()
	if err != nil {
		return nil, err
	}
	tables := &Result{
		Tables: &mesher.Tables[uint32]{
			Index:      make(map[uint32][]mesher.ModelID, len(states.ByID)),
			FullHeight: map[mesher.ModelID]mesher.ModelID{},
		},
		Metadata: blockstate.Metadata{
			Version:   engine.Assets.BuildInfo.Version,
			Renderers: map[asset.ID]blockstate.Renderer{},
		},
	}
	for name, profile := range engine.Assets.Renderers.Blocks {
		tables.Renderers[name] = blockstate.Renderer{Kind: profile.Kind, Family: profile.Family}
	}
	groups := map[string]uint32{}
	buckets := map[shapeBucket][]mesher.ModelID{}
	// 哈希不稠密，用 map 索引；排序只为让同一套资源的编号可复现。
	for _, javaHash := range slices.Sorted(maps.Keys(states.ByID)) {
		binding := states.ByID[javaHash]
		meshes, err := engine.bakeParts(binding.Java)
		if err != nil {
			tables.Skipped = append(tables.Skipped, blockstate.SkippedState{Binding: binding, Reason: err.Error()})
			continue
		}
		profile := engine.Assets.Renderers.Blocks[binding.Java.Name]
		group := cullGroup(binding.Java, profile.Kind, profile.Family, groups)
		var full []geometry.Mesh
		if profile.Kind == asset.RendererFluid {
			full, err = engine.bakeParts(binding.Java.With(map[string]string{"level": "8"}))
			if err != nil {
				return nil, err
			}
		}
		// 整个状态成功后再编号，失败状态不会留下部分索引或无引用的 Shape。
		ids := make([]mesher.ModelID, 0, len(meshes))
		for _, mesh := range meshes {
			ids = append(ids, intern(tables.Tables, mesh, group, buckets))
		}
		tables.Index[javaHash] = ids
		if len(full) != 0 {
			tables.FullHeight[ids[0]] = intern(tables.Tables, full[0], group, buckets)
		}
	}
	return tables, nil
}

func (engine *Builder) bakeParts(state blockstate.State) ([]geometry.Mesh, error) {
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
		mesh, err := bake.Bake(
			resources.FluidPreview(state, profile.Family),
			asset.Apply{},
			state,
			engine.materials,
		)
		// 只有真正位于格子边界的流体面才允许邻居剔除。
		for side, covers := range meshing.BoundaryCoverage(mesh) {
			for _, cover := range covers {
				mesh.Quads[cover.Quad].CullFace = geometry.CullFace(side + 1)
			}
		}
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
		mesh, err := bake.Bake(
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

func cullGroup(state blockstate.State, kind asset.RendererKind, family string, groups map[string]uint32) uint32 {
	// 名称规则只在建表时消费，形状去重也必须区分消面语义。
	key := ""
	if kind == asset.RendererFluid {
		key = "fluid:" + family
	} else if strings.Contains(state.Name.Path(), "glass") {
		key = "glass:" + string(state.Name)
	}
	if key == "" {
		return 0
	}
	if groups[key] == 0 {
		groups[key] = uint32(len(groups) + 1)
	}
	return groups[key]
}
