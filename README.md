# neomega-viewer

`neomega-viewer` 把存储中的方块 ID 转换成静态几何，并提供单方块和子区块两个网页入口。Go 解释方块状态、选择模型、处理纹理引用与网格；TypeScript 和 Three.js 显示已经生成的顶点。单方块直接解析一个方块，子区块直接读取一个 16×16×16 的 Palette，即用调色板索引压缩保存方块 ID 的存储。两者共享方块解析器 `blockmesh`。

方块 ID 本身不能说明外形。同一个名称可能因属性选择不同模型；栅栏还会同时使用多个模型。因此，解析保留资源之间的关系：**ID → Java 方块状态 → variant / multipart → model → texture → 顶点**。资源表保留模型树，请求具体状态时才生成几何。将这些几何放进子区块后，再根据邻居的实际形状剔除和合并表面。

## 先运行浏览器

需要两份配套数据：`neomega-blocks` 将存储的 ID 转成 Java 状态，`neomega-assets` 提供对应版本的资源。默认适配器使用 **NEMC 的 uint32 网络哈希**，Java 目标为 **1.21.4**。ID 来自 Go 预设及目录接口，前端保存并回传它，不计算哈希或执行方块转换。

需要 Go 1.25.5 或以上、Node.js 22.12 或以上，以及支持 WebGL 2 的浏览器。本地模块按相邻目录组织：

```text
new-world/
├─ neomega-assets/
├─ neomega-blocks/
├─ neomega-celler/
├─ neomega-nbt/
└─ neomega-viewer/
```

如果还没有资源，在 `neomega-assets` 中生成一次：

```sh
go run ./cmd/neomega-assets build -version 1.21.4
```

viewer 只读取交付目录 `neomega-assets/dist/final`，无须重新读取 JAR。默认适配器会检查版本，避免把不同版本的状态和模型混用。资源更新方式见 [neomega-assets README](../neomega-assets/README.md)。

然后在 `neomega-viewer` 中构建网页并启动服务：

```sh
npm --prefix web ci
npm --prefix web run build
go run ./cmd/neomega-viewer
```

两个入口有不同的观察目的：

| 页面 | 用途 |
|---|---|
| <http://127.0.0.1:2410> | 搜索方块、选择状态，检查单个模型与静态预览属性 |
| <http://127.0.0.1:2410/subchunk.html> | 观察多个形状的交界，开关剔除与合并，上传方块层、下载二进制网格 |

子区块页提供石头、玻璃、半砖、草木和混合场景。勾选“三角网格”后，可以直接比较合并前后的拓扑。“按位置变化模型”让不同位置选择不同的加权候选；旋转和镜像差异会影响哪些面能够合并。

资源或网页位于其他地方时，显式传入路径：

```sh
go run ./cmd/neomega-viewer \
  -assets /path/to/dist/final \
  -web /path/to/web/dist \
  -listen 127.0.0.1:2410
```

开发网页时，保持 Go 服务运行，在另一终端执行 `npm --prefix web run dev`，然后打开 <http://127.0.0.1:2411> 或其 `/subchunk.html` 页面。Vite 将 API、图集和资源声明请求代理到 Go 服务。

## 一个方块怎样变成几何

要让单方块和子区块使用相同的解释规则，首先需要一个没有场景调度职责的解析器。`blockmesh.Resolver` 组合以下步骤，并共享只读的资源、模型继承缓存和材质库。模型通过 UV（纹理上的二维坐标）指定每个表面点采样的位置：

```text
uint32 ID
  │ blockstate：查回 Bedrock 状态，转换成 Java 名称与属性
  ▼
Java 状态 + 静态预览属性
  │ model.Select：选择 variant，收集命中的 multipart 部件
  ▼
模型引用 + 旋转 + UV 锁定
  │ model.Resolver：解析父模型、合并纹理槽、跟随 #纹理别名
  ▼
模型元素与面
  │ geometry.Bake + material.Library
  ▼
局部四边形 geometry.Mesh
```

`variant` 按状态选择候选模型；`multipart` 分别判断条件，同时叠加命中的部件。加权候选只选一个，种子使这个选择可重复。父模型按模型 ID 缓存，原始模型树不会被展开成每个方块 ID 一份的巨大表。

几何使用一个方块为一单位的局部坐标，Y 向上，北向为 −Z。四边形保存几何法线、UV、颜色和材质。模型要求覆盖明暗方向时，另外保存光照法线，真实几何法线仍保持原义。`CullFace` 保存模型允许被邻居遮挡的方向；单方块入口保留这些面，子区块入口检查其实际覆盖情况。

## 形状、透明度与静态外观

半砖的半高来自模型几何，草的外形来自旋转的交叉平面。几何形状和贴图透明度需要分别处理：

| 表面类型 | 示例 | 绘制方式 |
|---|---|---|
| 不透明 `opaque` | 石头、半砖、栅栏 | 写入深度，同材质面合批 |
| 镂空 `cutout` | 树叶、草 | 丢弃透明像素，其余像素写入深度 |
| 半透明 `blend` | 玻璃、水 | 混合颜色，不写入深度，逐面参与远近排序 |

`material.Library` 根据图集像素分类。`resources.PreviewPolicy` 补充玻璃等语义规则，以及固定的草木、水和红石颜色。有厚度的镂空元素使用正面绘制，避免多画盒体的背面。独立薄片可以双面显示；草已有明确前后面时，分别绘制对应一侧，避免重复绘制。前端在每张纹理自己的图集区域内采样；即使一个面合并到跨越多个方块，也会重复纹理。

箱子、床和告示牌等方块的完整外观由游戏代码绘制，JAR 未必包含完整模型。`neomega-assets` 的 `blockRenderers.json` 说明类别并补入社区静态模板，viewer 通过同一模型接口解析它们。图集 PNG 单独加载，不随每个网格重复传输。

这里的静态指**给定状态和选项后，外观保持不变**。箱子使用固定姿态，告示牌展示牌体，动画纹理使用固定帧。库不接收方块实体的实时外观或动画状态，也不计划增加这些能力。水和岩浆使用静止预览表面；子区块中的同类流体上方还有流体时，补满当前格的液面高度。这不是游戏的流体曲面模拟。

PyMCTranslate 的主方块输出可能没有附加水层。当模型条件需要 `waterlogged` 而转换结果缺少它时，预览补为 `false`。修改该属性只影响模型选择，不会自行添加水层。`air` 等不可见类别返回空几何；NEMC 的 `micro_block` 视为空气。`moving_piston` 缺少决定移动内容的数据，会返回无法确定静态外形的错误。

## 多个方块放在一起时

直接叠加 4096 份方块网格，会保留大量完全看不见的内面。子区块入口先读取所有逻辑位置，再解析和缓存局部形状。这里不能只遍历 `IterNonDefault`：Palette 的默认值可以是石头，默认填满的格子同样需要参与渲染。

Bedrock ID 没有编码栅栏和玻璃板的连接方向。默认的 `resources.Connections` 根据邻居类别、朝向和实际完整不透明侧面选择这些部件。墙等已有连接状态沿用转换结果。`KeepConnections` 可以保留存储状态，`Engine.Connections` 可以替换整套静态连接策略。这些预览规则不等同于游戏全部放置与连接规则。

确定形状后，`meshing` 分两步减少面：

1. **剔除实际被盖住的部分。** 模型声明可剔除、且位于格子边界的轴对齐矩形，才会检查邻居。邻居的完整不透明面可以遮挡；多个矩形的并集也可以形成覆盖。石头紧贴底半砖时，只裁掉下半部分，并为保留部分重新插值 UV。斜面和非矩形保守保留。
2. **合并相邻矩形。** 在同一平面上按实际矩形边界建立二维网格，贪婪地扩展连续区域。只有材质、颜色、法线、光照方向、UV 变化率和重复相位兼容时才合并。合并后的 UV 延伸到多个纹理周期，保持每个方块原有的纹理尺寸。

透明面有额外规则。相同玻璃、同类流体可以移除实际重叠的内面；不同玻璃保留界面。玻璃和树叶不会作为不透明遮挡物盖掉后面的石头。半透明面不参与大矩形合并，前端仍可按四边形排序。逐面排序不保证任意相交半透明表面的严格顺序。

默认所有格子使用同一个模型选择种子，便于检查合并结果。设置 `VaryModels=true` 后，种子再结合局部位置选择候选，保留随机旋转和镜像。即使名称相同，UV 不兼容的面也不会强行合并。

独立贪婪矩形可能形成 T 形接缝，即较短边接在另一条长边的中间；算法不额外缝合这些边。外边界按暴露面处理；组装地图时还需要提供相邻子区块的边界信息。子区块解析不创建地图或 worker，地图加载与卸载属于另一个入口的调度职责。

## 为什么把网格传输单独分开

单方块的 JSON 便于查看来源和模型。子区块则有更多顶点，数值文本解析和临时数组会产生额外开销。因此子区块通过 `meshwire` 输出 **NVMS v1 二进制**：材质表使用一小段 JSON，顶点属性和索引使用对齐的小端数值数组。

浏览器的 `decodeMesh` 在常见的小端平台上为响应 `ArrayBuffer` 建立 `Float32Array`、`Uint32Array` 视图。Three.js 适配层直接保留这些视图；GPU 上传仍由 Three.js 完成。协议不依赖 HTTP，同一份数据也可以写入 `.nvmesh` 文件。布局与解析接口见 [meshwire/README.md](meshwire/README.md)。

网格合并与编码解决的是不同问题。以 Java 1.21.4 资源、当前 NEMC preset、默认种子且关闭位置变化的满石头层为例：

| 开启的处理 | 输出四边形 | NVMS 字节数 |
|---|---:|---:|
| 均关闭 | 24,576 | 6,095,048 |
| 仅面剔除 | 1,536 | 381,128 |
| 面剔除与合并 | 6 | 1,688 |

这些是未压缩的文件大小。下面的命令在仓库根目录生成表中三份文件，并输出各阶段面数：

```sh
go run ./examples/subchunk -scene solid -cull=false -greedy=false -mesh /tmp/solid-raw.nvmesh
go run ./examples/subchunk -scene solid -greedy=false -mesh /tmp/solid-culled.nvmesh
go run ./examples/subchunk -scene solid -mesh /tmp/solid.nvmesh -blocks /tmp/solid.nvblocks
wc -c /tmp/solid-raw.nvmesh /tmp/solid-culled.nvmesh /tmp/solid.nvmesh
```

更换资源、模型选择或编码版本后，应重跑命令核对数量。

## 直接使用 Go 库

HTTP 服务只是组合入口。自己的 Go 应用可以打开一次解析器，交给两个相互独立的构建器：

```go
parser, err := blockmesh.Open(blockmesh.OpenOptions{
    AssetsDir: "../neomega-assets/dist/final",
})
if err != nil {
    return err
}
blockBuilder := singleblock.New(parser)
sectionBuilder := subchunk.New(parser)
```

这些包的导入前缀为 `neomega-viewer/`。单方块仍直接接收一个 ID：

```go
block, err := blockBuilder.Build(blockID, singleblock.Options{
    Seed: 0,
    Properties: map[string]string{"north": "true"},
})
```

`block.Binding.Java` 保留 ID 的转换结果，`block.State` 是应用预览选项后的状态。现成示例会先通过 `neomega-blocks` 将 Java 文本转换成目标 ID，然后输出 JSON：

```sh
go run ./examples/single-block -java 'minecraft:oak_slab [type=bottom]' > /tmp/slab.json
```

子区块直接接收调用者已有的 `*layer_storage.Palette[uint32]`。这个类型来自 `neomega-celler`，Go 导入路径为 `celler/layer_storage`：

```go
// layer: *layer_storage.Palette[uint32]
result, err := sectionBuilder.Build(layer, subchunk.Options{})
if err != nil {
    return err
}
encoded, err := meshwire.Encode(result.Mesh.Buffers())
```

构建期间调用者应保持 Palette 不变。读取和网格处理不会修改它。解析器及连接策略配置后视为只读，同一个构建器可以并发处理不同输入。

需要跨工具交换方块层时，`subchunk.EncodeIDs` 输出恰好 **4096 个小端 uint32 ID，共 16,384 字节**。索引为 `(x << 8) | (z << 4) | y`，即 Y 最快、Z 次之、X 最慢。`subchunk.ParseIDs` 将其还原成 Palette。这个 `.nvblocks` 格式保存逻辑值，不依赖 Palette 的内部内存或原生 dump 布局。

例如，生成后在网页上传 `/tmp/solid.nvblocks`，或在命令行再次读取它：

```sh
go run ./examples/subchunk -input /tmp/solid.nvblocks -mesh /tmp/reloaded.nvmesh
```

## 复用前端绘制

外部页面需要自行管理网络和场景时，可使用 `web` 的 `@neomega/viewer/render` 导出。它只依赖 Three.js，不依赖演示页、方块目录、地图或 worker：

```sh
npm install /path/to/neomega-viewer/web
```

Three.js 作为 peer dependency 由宿主共享。本地目录链接也有开发用的 Three.js，Vite 宿主应加入 `resolve: { dedupe: ["three"] }`，让场景与绘制模块使用同一个实例。

获取二进制网格后，可以放入自己的 Three.js 场景：

```ts
import { createMeshObject, decodeMesh, loadAtlas } from "@neomega/viewer/render";

const response = await fetch("/api/subchunks/solid/mesh");
if (!response.ok) throw new Error(await response.text());
const data = decodeMesh(await response.arrayBuffer());
const atlas = await loadAtlas("/assets/atlas.png");
const mesh = createMeshObject(data, atlas);
scene.add(mesh.object);

// 不再使用时释放本对象的缓冲与材质；共享图集由调用者管理。
scene.remove(mesh.object);
mesh.dispose();
atlas.dispose();
```

`createMeshObject` 也接受单方块 JSON 的数值数组。希望一并复用相机和视口时，使用 `MeshViewport`，再调用 `set(data)`；`BlockViewport` 保留为原有名称的兼容导出。

## 接口与扩展位置

更换 ID 的含义，应替换 `blockmesh.OpenOptions.Resolver`，实现 `blockstate.Resolver`。调用者负责让返回的 Java 状态与资源版本匹配。固定颜色和材质语义通过 `material.Policy` 替换，连接规则通过子区块构建器的 `Connections` 替换。

其他变化可以按职责定位：

| 要解决的问题 | 模块 |
|---|---|
| ID 对应哪个 Java 状态？ | `blockstate` |
| 状态命中哪些部件，模型怎样继承？ | `model` |
| 一个状态怎样变为局部几何？ | `blockmesh`、`geometry`、`material`、`resources` |
| 如何构建单个方块？ | `singleblock` |
| 如何读取整个 Palette 并处理邻接？ | `subchunk` |
| 哪些表面被覆盖，哪些矩形能够合并？ | `meshing` |
| 几何怎样编码和解码？ | `meshwire`、`web/src/render/binary.ts` |
| 哪些代码只是演示？ | `demo`、`httpapi`、`web/src/browser`、`web/src/subchunk` |

演示服务提供以下接口：

| 请求 | 返回内容 |
|---|---|
| `GET /api/blocks` | JSON 名称分组、示例 ID、版本和图集地址 |
| `GET /api/states?name=minecraft:oak_fence` | JSON 状态列表与真实 ID |
| `GET /api/state?id=<uint32>` | JSON ID 转换结果 |
| `GET /api/block?id=<uint32>` 或 `POST /api/block` | JSON 单方块来源、模型和几何 |
| `GET /api/subchunks` | JSON 子区块示例目录 |
| `GET /api/subchunks/{scene}/mesh` | NVMS 二进制几何 |
| `GET /api/subchunks/{scene}/blocks` | `.nvblocks` 逻辑方块层 |
| `POST /api/subchunk` | 提交 `.nvblocks` 原始字节，返回 NVMS |
| `GET /assets/atlas.png` | 与材质区域配套的 PNG |

子区块网格接口接受 `cull`、`greedy`、`links`、`vary` 布尔查询参数，以及 `seed`。前三项默认开启，`vary` 默认关闭。统计值通过 `X-Neomega-Mesh-Stats` 响应头返回；完全移除的源面与局部裁切的源面分别计数。局部裁切可能产生多个保留片段，因此“原始面减去移除面”不一定等于“剔除后面数”。

上传已有方块层时，直接发送二进制正文：

```sh
curl -fsS http://127.0.0.1:2410/api/subchunk \
  -H 'Content-Type: application/octet-stream' \
  --data-binary @/tmp/solid.nvblocks \
  -o /tmp/uploaded.nvmesh
```

输入格式或参数无效返回 400；无法确定静态几何返回 422，错误正文使用 JSON。未知示例返回 404。

## 验证解析与传输

不依赖生成资源的小型测试检查模型继承、条件选择、旋转、UV、覆盖并集与孔洞、局部裁切、贪婪兼容条件和二进制布局：

```sh
go test ./...
go vet ./...
npm --prefix web test
npm --prefix web run build
```

加入真实资源后，可以验证完整 preset、全部演示场景以及 HTTP 上传和二进制输出：

```sh
NEOMEGA_ASSETS_DIR="$PWD/../neomega-assets/dist/final" \
  go test -race -v ./... -count=1 -timeout=90s
```

Java 1.21.4 与当前 NEMC preset 的单方块全表检查生成 15,155 个静态结果，另有 1 个 `moving_piston` 明确返回静态能力边界。实际数量以更换数据后重新运行的输出为准。

要检查 Go 编码器与 TypeScript 解码器的配合，可使用前面生成的满石头网格：

```sh
NEOMEGA_MESH_FIXTURE=/tmp/solid.nvmesh npm --prefix web test
```

该检查验证面数、范围、重复 UV，以及解码器和 Three.js 适配层对原始数值缓冲的共享。

## 来源与许可

原创代码采用 [MIT](LICENSE)，Three.js 使用 MIT。Palette 来自相邻的 `neomega-celler`。模型和纹理由 `neomega-assets` 提供，包含 Minecraft 资源和 `zardoy/mc-assets` 社区模板；生成网格不会改变素材权利来源。部署时应保留交付目录中的 `THIRD_PARTY_NOTICES.txt`。转换数据有独立许可，不能由本项目 MIT 覆盖。详见 [NOTICE.md](NOTICE.md)。
