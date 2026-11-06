package main

import (
	"math"
	"sync"

	"neomega-blocks/ir"
	"neomega-viewer-core/viewer/blockdata"
	"neomega-viewer-core/viewer/dispatcher"
	"neomega-viewer/source"
)

// 未打开存档时的默认场景：玻璃海上升起一枚海晶灯莫比乌斯环。
// 它是普通的方块来源，与存档共用区块加载和绘制路径，页面只需换上线描外观。
const (
	sunriseSeaLevel  = 64
	sunriseRingR     = 46.0
	sunriseRingWidth = 15.0
	// 环心高出海面半径的一半，圆环四分之三在海上、四分之一在海下。
	sunriseRingY = sunriseSeaLevel + sunriseRingR/2
	// 扭转点的圆周角。放在右上方，正面看仍是 O，只有这一处收成细线透露出它是单面的。
	sunriseTwistAngle = math.Pi / 7
	// 长焦取景：相机退得远，近处的方块才不会大到破坏涟漪的细腻感。
	sunriseCameraY = sunriseSeaLevel + 30.0
	sunriseCameraZ = 340.0
	sunriseFov     = 28.0
	// 海面是一片椭圆，外缘逐格消散成空气，椭圆中心偏向相机。
	sunriseSeaCenterZ = 92.0
	sunriseSeaRadiusX = 190.0
	sunriseSeaRadiusZ = 170.0
)

// Backdrop 告诉页面如何取景以及场景的几何锚点；初始位置仍由 Info.Position 给出。
type Backdrop struct {
	Look     [3]float64 `json:"look"`
	Fov      float64    `json:"fov"`
	Center   [3]float64 `json:"center"`
	Radius   float64    `json:"radius"`
	SeaLevel float64    `json:"seaLevel"`
	SeaEnd   float64    `json:"seaEnd"`
	Twist    [3]float64 `json:"twist"`
	Lanterns int        `json:"lanterns"`
	Glass    int        `json:"glass"`
}

// sunriseCanvas 累积方块，最后一次性写成 section。
type sunriseCanvas struct {
	sections map[dispatcher.SectionPos]*blockdata.Blocks
	cells    map[[3]int32]struct{}
}

func (c *sunriseCanvas) set(x, y, z int32, state ir.IRBlock) {
	pos := dispatcher.SectionPos{X: x >> 4, Y: y >> 4, Z: z >> 4}
	blocks := c.sections[pos]
	if blocks == nil {
		blocks = blockdata.Uniform(4096, ir.IRBlock("minecraft:air []"))
		c.sections[pos] = blocks
	}
	// 与 cube.Layout 一致的 X-Z-Y 顺序。
	blocks.Set(int(x&15)<<8|int(z&15)<<4|int(y&15), state)
	c.cells[[3]int32{x, y, z}] = struct{}{}
}

type sunriseScene struct {
	sections source.MemorySections
	backdrop Backdrop
}

var sunrise = sync.OnceValue(func() sunriseScene {
	canvas := &sunriseCanvas{
		sections: map[dispatcher.SectionPos]*blockdata.Blocks{},
		cells:    map[[3]int32]struct{}{},
	}
	paintSea(canvas)
	glass := len(canvas.cells)
	paintMobiusRing(canvas)
	sections := make(source.MemorySections, len(canvas.sections))
	for pos, blocks := range canvas.sections {
		sections[pos] = &dispatcher.Section{Blocks: blocks}
	}
	return sunriseScene{
		sections: sections,
		backdrop: Backdrop{
			Look:     [3]float64{0, sunriseSeaLevel + 26, 0},
			Fov:      sunriseFov,
			Center:   [3]float64{0, sunriseRingY, 0},
			Radius:   sunriseRingR,
			SeaLevel: sunriseSeaLevel,
			SeaEnd:   sunriseSeaCenterZ - sunriseSeaRadiusZ,
			Twist: [3]float64{
				sunriseRingR * math.Cos(sunriseTwistAngle),
				sunriseRingY + sunriseRingR*math.Sin(sunriseTwistAngle),
				0,
			},
			Lanterns: len(canvas.cells) - glass,
			Glass:    glass,
		},
	}
})

func paintMobiusRing(canvas *sunriseCanvas) {
	// 带子沿圆周走一圈只转半圈。按小于一格的步长扫描宽度和厚度，体素化后不留缝。
	lantern := ir.IRBlock("minecraft:sea_lantern []")
	const step = 0.45
	steps := int(math.Ceil(2 * math.Pi * (sunriseRingR + sunriseRingWidth) / step))
	phase := math.Pi/2 - sunriseTwistAngle/2
	for i := range steps {
		angle := 2 * math.Pi * float64(i) / float64(steps)
		twist := angle/2 + phase
		radialX, radialY := math.Cos(angle), math.Sin(angle)
		for v := -sunriseRingWidth / 2; v <= sunriseRingWidth/2; v += step {
			for t := -0.5; t <= 0.5; t += step {
				radial := sunriseRingR + v*math.Cos(twist) - t*math.Sin(twist)
				depth := v*math.Sin(twist) + t*math.Cos(twist)
				canvas.set(
					int32(math.Floor(radial*radialX)),
					int32(math.Floor(sunriseRingY+radial*radialY)),
					int32(math.Floor(depth)),
					lantern,
				)
			}
		}
	}
}

func paintSea(canvas *sunriseCanvas) {
	for x := int32(-sunriseSeaRadiusX); x <= sunriseSeaRadiusX; x++ {
		for z := int32(sunriseSeaCenterZ - sunriseSeaRadiusZ); z <= sunriseSeaCenterZ+sunriseSeaRadiusZ; z++ {
			fx, fz := float64(x), float64(z)
			edge := math.Hypot(fx/sunriseSeaRadiusX, (fz-sunriseSeaCenterZ)/sunriseSeaRadiusZ)
			if edge >= 1 || !onRipple(fx, fz) || cellNoise(x, z, 1) < smoothstep(0.6, 1, edge) {
				continue
			}
			canvas.set(x, sunriseSeaLevel, z, seaGlass(x, z))
		}
	}
}

// onRipple 判断这一格是否落在涟漪线上。海面只由这些线构成，线之间是空气。
func onRipple(x, z float64) bool {
	return math.Abs(math.Mod(ripplePhase(x, z)+1000.5, 1)-0.5) < 0.17
}

func ripplePhase(x, z float64) float64 {
	// 平行的波列被低频场扭曲，再叠上以倒影为中心的同心环，形成指纹般的纹路。
	warp := 1.5*math.Sin(x*0.034+1.4*math.Sin(z*0.021)) +
		0.9*math.Sin(x*0.071-z*0.038+0.7) +
		0.5*math.Sin(x*0.13+z*0.05)
	rings := 2.4 * math.Exp(-math.Pow(math.Hypot(x, (z-110)*0.8)/120, 2)) * math.Sin(math.Hypot(x, (z-110)*0.8)*0.09)
	return z/6 + warp + rings
}

func seaGlass(x, z int32) ir.IRBlock {
	fx, fz := float64(x), float64(z)
	if reflectsRing(fx, fz) {
		if cellNoise(x, z, 3) < 0.7 {
			return "minecraft:white_stained_glass []"
		}
		return "minecraft:light_blue_stained_glass []"
	}
	// 环根部的海面被日光染暖，向外依次退到冷色；抖动让相邻色带互相渗透。
	distance := math.Hypot(fx, fz*0.55) + (cellNoise(x, z, 4)-0.5)*26
	switch {
	case distance < 34:
		return "minecraft:yellow_stained_glass []"
	case distance < 62:
		return "minecraft:orange_stained_glass []"
	case distance < 96:
		return "minecraft:light_blue_stained_glass []"
	case distance < 150:
		return "minecraft:cyan_stained_glass []"
	}
	return "minecraft:blue_stained_glass []"
}

func reflectsRing(x, z float64) bool {
	// 从默认视点镜像出的视线穿过这一格后打到环所在平面，命中圆环就是倒影。
	// 水面的起伏让视线左右摆动，倒影边缘因此碎成波纹。
	if z <= 0 || z >= sunriseCameraZ {
		return false
	}
	scale := sunriseCameraZ / (sunriseCameraZ - z)
	mirrorY := 2*sunriseSeaLevel - sunriseCameraY
	wobble := 7 * math.Sin(ripplePhase(x, z)*0.9+z*0.03)
	hitX := x*scale + wobble
	hitY := mirrorY + scale*(sunriseSeaLevel-mirrorY)
	radius := math.Hypot(hitX, hitY-sunriseRingY)
	return math.Abs(radius-sunriseRingR) < sunriseRingWidth/2+1.5
}

func cellNoise(x, z int32, salt uint32) float64 {
	h := uint32(x)*0x9e3779b1 ^ uint32(z)*0x85ebca77 ^ salt*0xc2b2ae3d
	h ^= h >> 15
	h *= 0x2c1b3c6d
	h ^= h >> 12
	h *= 0x297a2d39
	h ^= h >> 15
	return float64(h) / (1 << 32)
}

func smoothstep(from, to, value float64) float64 {
	t := math.Min(math.Max((value-from)/(to-from), 0), 1)
	return t * t * (3 - 2*t)
}

// applySunrise 让未打开存档的来源显示默认场景；它不持有句柄，Loaded 保持 false。
func applySunrise(opened *Source) {
	scene := sunrise()
	opened.reader = func() dispatcher.SectionSource { return scene.sections }
	opened.Info.Backdrop = &scene.backdrop
	opened.Info.Position = [3]float64{0, sunriseCameraY, sunriseCameraZ}
}
