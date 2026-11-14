package worldview

import (
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer/internal/startscene"
	"neomega-viewer/minimap"
)

func applySunrise(opened *Source) error {
	// 三维视图与小地图共享默认场景的只读方块，Loaded 仍表示是否打开了存档。
	terrain, err := minimap.NewTerrain(minimap.Overworld)
	if err != nil {
		return err
	}
	scene := startscene.Default()
	opened.reader = func() dispatcher.SectionSource { return scene.Sections }
	opened.minimap = &minimap.Options{
		Terrain: terrain,
		Reader:  opened.reader,
		Chunks:  scene.Sections.Chunks,
	}
	opened.Info.Backdrop = &scene.Backdrop
	opened.Info.Position = scene.Position
	return nil
}
