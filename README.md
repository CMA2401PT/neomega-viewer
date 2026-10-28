# neomega-viewer

`neomega-viewer` 把存储中的方块 ID 转换成可以绘制的静态几何，并提供一个浏览方块的网页。Go 负责解释方块状态、选择模型、处理继承与纹理引用，最后生成顶点。TypeScript 和 Three.js 负责显示这些顶点。

方块 ID 本身不能说明外形。同一个方块会因属性选择不同模型；栅栏还会同时使用多个模型。因此，解析过程保留资源之间的关系：**ID → Java 方块状态 → variant / multipart → model → texture → 顶点**。资源表保持模型树的结构，只在请求具体方块时生成几何。

## 先运行方块浏览器

浏览器需要两份能配套的数据：`neomega-blocks` 将实际存储的 ID 转成 Java 状态，`neomega-assets` 提供对应版本的资源。默认适配器使用 **NEMC 的 uint32 网络哈希**，Java 目标为 **1.21.4**。这些 ID 是已有数据中的标识，并非浏览器自行分配的序号。

本地开发使用下面的相邻目录。需要 Go 1.25.5 或以上，以及 Node.js 22.12 或以上；浏览器需要 WebGL 2。

```text
new-world/
├─ neomega-assets/
├─ neomega-blocks/
├─ neomega-nbt/
└─ neomega-viewer/
```

如果还没有资源，在 `neomega-assets` 中生成一次：

```sh
go run ./cmd/neomega-assets build -version 1.21.4
```

这会得到 `neomega-assets/dist/final`。运行 viewer 只需要这个交付目录，无须重新读取 JAR。资源生成和更新方式见 [neomega-assets README](../neomega-assets/README.md)。默认适配器会检查资源版本，避免将不同版本的状态和模型混用。

然后在 `neomega-viewer` 中构建网页并启动服务：

```sh
npm --prefix web ci
npm --prefix web run build
go run ./cmd/neomega-viewer
```

打开 <http://127.0.0.1:2410>。左侧搜索方块名称，右侧选择存储状态或输入网络哈希。静态预览属性可以改变半砖的上下位置、栅栏的连接方向等外观；它们不修改原始 ID 对应的状态。相机可以拖动、缩放，也可以切换正交投影。

资源或网页位于其他地方时，显式传入路径：

```sh
go run ./cmd/neomega-viewer \
  -assets /path/to/dist/final \
  -web /path/to/web/dist \
  -listen 127.0.0.1:2410
```

开发网页时，保持 Go 服务运行，在另一个终端执行 `npm --prefix web run dev`，然后打开 <http://127.0.0.1:2411>。Vite 会将 API 和图集请求代理到 Go 服务。

## 一个方块怎样变成几何

为了在命令行、网页和其他场景中复用同一套解析，解析库不依赖 HTTP，也不创建地图。单方块入口直接完成下面的组合：

```text
uint32 ID
  │ blockstate：查回 Bedrock 状态，转换成 Java 名称与属性
  ▼
Java 状态 + 静态预览属性
  │ model.Select：选择 variant，收集命中的 multipart 部件
  ▼
模型引用 + 旋转 + UV 锁定
  │ model.Resolver：解析父模型，合并纹理槽，跟随 #纹理别名
  ▼
模型元素与面
  │ geometry.Bake + material.Library：生成四边形和材质描述
  ▼
geometry.Mesh → Buffers → Three.js
```

`variant` 是按状态选择的一组候选模型。`multipart` 是分别判断条件、同时叠加的模型部件。加权候选只选取其中一个；`Seed` 让选择可重复，不引入随时间变化的外观。父模型解析结果按模型缓存，并由请求共享。

顶点使用一个方块为一单位的局部坐标，Y 向上，北向为 −Z。四边形保存几何法线、纹理 UV、颜色和材质。模型要求覆盖明暗方向时，单独保存光照法线，几何法线仍表示真实表面方向。`CullFace` 保留源模型指定的邻居方向；孤立方块没有邻居，因此会保留全部可见面。

## 为什么需要区分表面类型

形状和透明度是两件事。半砖通过模型元素得到半高几何；草通过旋转的平面得到交叉外形。它们都不能靠“给一个立方体换贴图”正确显示。

纹理透明度又有三种不同的绘制要求：

| 表面 | 示例 | 绘制方式 |
|---|---|---|
| 不透明 `opaque` | 石头、半砖、栅栏 | 写入深度，同材质面合批 |
| 镂空 `cutout` | 树叶、草 | 丢弃透明像素，其余像素写入深度 |
| 半透明 `blend` | 玻璃、水 | 混合颜色，不写入深度，逐面参与远近排序 |

`material.Library` 根据图集像素区分不透明、镂空和半透明。`resources.PreviewPolicy` 补充玻璃等语义规则，以及孤立预览使用的固定草木、水和红石颜色。草已有明确的前后面时，分别绘制对应一侧，避免重叠绘制。前端在每张纹理自身的图集区域内采样，保持像素风格并避免取到相邻纹理。

逐面排序适合孤立预览，不保证任意相交半透明表面的严格排序。邻居遮挡也不能仅根据 `opaque` 判断：半砖、栅栏和树叶各自的覆盖范围不同，子区块网格需要结合形状处理。

## 特殊方块如何得到静态外形

箱子、床、告示牌等方块的完整外观由游戏代码绘制，JAR 中未必有完整模型。`neomega-assets` 的 `blockRenderers.json` 说明它们的类别，并补入社区提供的固定外形。viewer 从同一套模型接口解析这些静态模板。

静态在这里指**给定状态和预览选项后，外形保持不变**。箱子保持固定开合姿态，告示牌展示牌体，动画纹理使用图集中的固定帧。库不接收方块实体的实时外观或动画状态，也不计划增加这类能力。水和岩浆使用 `resources.FluidPreview` 提供孤立的静止表面，液面高度由已有 `level` 属性决定。

PyMCTranslate 的主方块输出可能不含附加水层。当模型条件确实需要 `waterlogged`、转换结果又缺少它时，预览补为 `false`。切换该属性只影响模型选择，不会自行添加水几何。草木颜色也不查询群系；需要另一套固定颜色时，可以替换材质策略。

`air` 和其他不可见类别返回空几何。NEMC 的 `micro_block` 按转换表视为空气。`moving_piston` 仅凭方块状态不能确定正在移动的内容，因此返回“没有固定静态外形”的错误，网页会显示原因。

## 不启动服务，直接使用 Go 库

HTTP 只是演示程序的传输方式。其他 Go 代码可以打开一次资源，重复调用独立的单方块入口：

```go
engine, err := singleblock.Open(singleblock.OpenOptions{
    AssetsDir: "../neomega-assets/dist/final",
})
if err != nil {
    return err
}
block, err := engine.Build(blockID, singleblock.Options{
    Seed: 0,
    Properties: map[string]string{
        "north": "true", // 例如，为栅栏选择一个固定连接外观
    },
})
if err != nil {
    return err
}
quads := block.Mesh           // 保留四边形及 CullFace，可供其他网格算法使用
packet := block.Packet()      // 转为带 JSON 类型标记的顶点与材质缓冲
_ = quads
_ = packet
```

导入路径为 `neomega-viewer/singleblock`。`block.Binding.Java` 保留 ID 的转换结果，`block.State` 是应用预览选项后的实际状态。调用者应使用模型接受的属性值。资源、模型缓存和材质库加载后应视为只读；同一个 `Engine` 可以并发生成多个方块。

还没有网络哈希时，可以先借助 `neomega-blocks` 将 Java 文本转换为目标方块。可运行示例位于 [examples/single-block/main.go](examples/single-block/main.go)：

```sh
go run ./examples/single-block \
  -java 'minecraft:oak_slab [type=bottom]' \
  > /tmp/slab.json
```

示例先取得目标方块的 `Hash()`，再生成半砖几何并输出 JSON。转换可能损失源格式独有的属性；这与浏览器修改一个固定预览外观是不同的操作。

## 按需要复用与扩展

更换输入 ID 的含义，不应连带改写模型解析。为 `singleblock.OpenOptions.Resolver` 提供一个实现了 `blockstate.Resolver` 的适配器，即可把自己的 uint32 ID 转成 `blockstate.Binding`。使用自定义适配器时，调用者负责让 Java 状态与所选资源版本匹配。固定颜色和材质语义通过 `material.Policy` 替换。

需要更细的控制时，可以直接组合较低层模块：

| 需要解决的问题 | 模块 |
|---|---|
| ID 对应哪个 Java 状态？ | `blockstate` |
| 状态命中哪些部件？父模型和纹理槽怎样继承？ | `model` |
| 纹理怎样绘制、如何着色？ | `material`、`resources` |
| 模型怎样变为局部四边形与顶点？ | `geometry` |
| 如何完成一次孤立方块解析？ | `singleblock` |
| 怎样列出预设状态、通过网页选择？ | `catalog`、`httpapi`、`web/src/browser` |

如果只需要前端绘制，也可以复用 `web` 的 `@neomega/viewer/render` 导出。它只依赖 Three.js，不依赖浏览器演示页、网络接口或方块列表。其他 TypeScript 工程可将 `web` 作为本地包依赖：

```sh
npm install /path/to/neomega-viewer/web
```

Three.js 作为 peer dependency 由宿主工程共享。使用本地目录链接时，开发目录中也有 Three.js；Vite 工程应在自己的配置中加入 `resolve: { dedupe: ["three"] }`，让场景和绘制模块使用同一个实例。

有现成 Three.js 场景时，将已经生成的 `MeshData` 接入它：

```ts
import { createMeshObject, loadAtlas } from "@neomega/viewer/render";
import type { MeshData } from "@neomega/viewer/render";

const atlas = await loadAtlas("/assets/atlas.png");
// data: MeshData，由调用者从文件、HTTP 或其他来源取得。
const mesh = createMeshObject(data, atlas);
scene.add(mesh.object);

// 不再使用时释放本对象的缓冲和材质；图集由调用者管理。
scene.remove(mesh.object);
mesh.dispose();
atlas.dispose();
```

希望连相机、网格和视口一起复用时，使用 `BlockViewport`。这两个入口均不创建地图或 worker。

单方块入口不需要构造一个“只有一个方块的世界”。子区块应另有直接读取 `*layer_storage.Palette[uint32]` 的入口，复用状态与模型解析，在自己的边界内处理邻居遮挡、形状覆盖和网格合并。地图入口再复用子区块解析，负责 worker、加载与卸载。这些边界保证地图调度不会进入单方块或子区块的基本解析流程；本仓库目前提供的是单方块入口。

## 通过 HTTP 取得相同结果

外部页面需要从 Go 取得几何时，可使用 `httpapi.New`，也可以只使用库并编写自己的协议。演示服务提供以下接口：

| 请求 | 用途 |
|---|---|
| `GET /api/blocks` | 版本、图集地址和按 Bedrock 名称分组的目录 |
| `GET /api/states?name=minecraft:oak_fence` | 某名称下的真实 ID、Bedrock 状态与 Java 转换结果 |
| `GET /api/state?id=<uint32>` | 查询一个 ID 的转换结果 |
| `GET /api/block?id=<uint32>` | 使用默认选项生成几何 |
| `POST /api/block` | 提交 `id`、`seed` 和静态 `properties` 生成几何 |
| `GET /assets/atlas.png` | 与顶点中的图集区域配套的 PNG |

例如，在浏览器控制台查询栅栏状态，再将返回的一个 ID 放入 POST 请求：

```js
const states = await fetch("/api/states?name=minecraft:oak_fence").then(r => r.json());
const block = await fetch("/api/block", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    id: states[0].id,
    seed: 0,
    properties: { north: "true" },
  }),
}).then(r => r.json());
console.log(block.geometry);
```

响应同时包含来源状态、实际预览状态、选中的模型和顶点缓冲。查询参数或 JSON 无效时返回 400；无法生成几何时返回 422，并在 `error` 中说明原因。

## 验证解析结果

小型测试使用自行构造的模型和像素，检查继承、变体、多部件条件、旋转、UV 锁定、面方向和透明分类：

```sh
go test ./...
go vet ./...
npm --prefix web run build
```

要验证与真实转换表的配合，需要给测试一份已经生成的资源。下面的命令遍历完整预设，也检查 HTTP 输出及代表性的玻璃、半砖、栅栏、草、火焰、箱子、床、悬挂告示牌、树叶和流体：

```sh
NEOMEGA_ASSETS_DIR="$PWD/../neomega-assets/dist/final" \
  go test -race -v ./... -count=1 -timeout=90s
```

使用 Java 1.21.4 资源和当前 NEMC preset，该检查生成 15,155 个状态的静态结果，另有 1 个 `moving_piston` 状态明确返回静态能力边界。数量来自 `singleblock` 的全表测试；更换 palette 后应以重新运行的输出为准。

## 来源与许可

原创代码采用 [MIT](LICENSE)。Three.js 使用 MIT 许可。模型和纹理由 `neomega-assets` 提供，其中包含 Minecraft 资源和 `zardoy/mc-assets` 的社区静态模板；其权利来源不会因生成顶点而改变。部署资源时应保留交付目录中的 `THIRD_PARTY_NOTICES.txt`。`neomega-blocks` 的转换数据也有独立来源，不能用本项目的 MIT 许可覆盖。详见 [NOTICE.md](NOTICE.md)。
