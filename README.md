# neomega-viewer

`neomega-viewer` 把数值方块层转换成静态几何。`mesher` 是连接方块数据与绘制端的可复用窄腰：调用者提供几何表和数值 block ID，它查表取得形状，再按邻居覆盖剔面、合并。`mesher.Mesher[B]` 的 B 可以是 uint32、uint64 或它们的自定义类型，库只把 ID 当查表键。

`mesher/` 自带 `geometry`、`material`、`meshing` 子包，分别负责几何与传输、表面材质、剔面与合并。整个子树除 Go 标准库外没有外部依赖。子区块入口接收 4096 个数值 block ID 的切片，存储类型由调用方选择。Java 状态、模型与图集解析留在外层，其他 ID 系统也可以提供自己的表并复用相同入口。

Java 适配层为这条入口准备数据。**生成侧**枚举完整 Java 状态、选模型、解析纹理、烘焙几何，产出 `javaHash → []ModelID` 和 `ModelID → Shape` 两张表。Java hash 是方块名加全部属性先 `Tidy()`、再 `Hash()` 得到的 uint32。模型部件被许多状态共用，所以几何去重后共享。**运行侧**只查表与拼装：单方块返回 Mesh，16×16×16 的子区块返回几何与统计。连接方向、半砖位置、红石颜色等已在生成时定死；流体高度切换和面剔除依赖邻居的数值几何信息。

两张表保存为固定小端的 NVGT 二进制。加载文件或 embed 是调用者的职责，mesher 构造时只接收 `*Tables[B]`。Java 演示加载器提供内嵌资源的便捷入口。

viewer 的输入是完整的 Java 状态，本身不需要修复。从 NEMC 等其他表达转换过来的状态可能丢失连接方向、楼梯拐角或门的部分属性，修复属于转换一侧，由 [neomega-converter](../neomega-converter) 的 `fixer` 负责，调用者修好后再把 hash 层交给 viewer。

含水台阶这类方块没有自己的模型，画面是方块本体叠上一格水。几何表只认单层的 hash（JavaBlockHashRSL），所以带 `waterlogged` 的输入要先经 `splitlayer` 拆成前景层和水层，再交给 mesher 的多层入口。

陶罐、旗帜这类方块的外观取决于 block entity NBT，附魔台上的书是动态效果，静态几何画不出来。`dispatcher` 把一个子区块按方块类型分到 mesher、effector、block actor 三条路，`merger` 再把三份结果打包成一个 NVFR 快照。设计见 [渲染分发方案.md](渲染分发方案.md)。

浏览器和 HTTP 服务不是库的一部分，是演示它怎么用的示例。

## 目录职责

复用几何构建时使用 `mesher/` 及其子包。演示代码集中在 `demo/`。只有生成器需要模型解析和图集采样，这些实现放在 `scripts/internal/`，由 Go 的 internal 边界限制引用。

```text
neomega-viewer/
├─ mesher/             # 泛型表、NVGT 编解码、单块与子区块入口
│  ├─ geometry/       # Shape、Mesh、Quad、绘制缓冲与 NVMS 编解码
│  ├─ material/       # 表面材质，纹理名为 string
│  └─ meshing/        # 剔面、裁切和合并
├─ blockstate/         # 上层 Java 身份与响应元数据
├─ splitlayer/         # 把含水方块拆成前景层与水层
├─ dispatcher/         # 按路由表把方块分给 mesher、effector、block actor，并串起整条流程
├─ effector/           # 输出「某格放某特效」的消息
├─ blockactor/         # 按 NBT 生成几何，每种方块一个子包（如 decoratedpot/）
├─ merger/             # NVFR 快照容器的编解码
├─ web/render/         # 前端渲染核心：NVMS/NVFR 解码、材质、视口、特效
├─ resources/          # 入库的几何表、Java 元数据及 embed
├─ demo/
│  ├─ blockmesh/       # Java 演示加载器，组合表与身份
│  ├─ singleblock/     # 调用 mesher 并补 Java 响应元数据
│  ├─ subchunk/        # .nvblocks 方块层读写
│  ├─ cmd/neomega-viewer/
│  ├─ httpapi/
│  ├─ catalog/         # 浏览器选择器目录
│  ├─ scenes/          # 示例场景
│  ├─ examples/        # 命令行示例
│  ├─ integration/     # 使用目录和场景的集成测试
│  └─ web/             # 演示网页：面板、场景选择、构建配置
└─ scripts/
   ├─ gen-geometry/    # 生成命令
   └─ internal/        # tablegen、bake、model、materials、resources
```

依赖方向为演示与生成脚本依赖 `mesher`，`mesher` 不反向依赖它们。内部依赖为 `geometry → material`、`meshing → geometry + material`。`catalog` 的分组和示例 ID 服务于浏览器选择器，因此属于演示。几何类型上的 NVMS 编解码可直接用于文件或网络。根目录的 `resources` 保存生成产物；生成时的预览规则在 `scripts/internal/resources`。

## 直接使用 Go 库

几何使用者需要先取得表，再创建一个 Mesher。所有包的导入前缀为 `neomega-viewer/`。下面的完整示例读取 uint32 表，将一个方块写成 NVMS 二进制；ID 来自调用者，必须存在于表中：

```go
package main

import (
    "flag"
    "log"
    "os"

    "neomega-viewer/mesher"
)

func main() {
    path := flag.String("tables", "resources/geometry.nvtables", "geometry table")
    id := flag.Uint64("id", 0, "block ID in the table")
    flag.Parse()
    file, err := os.Open(*path)
    if err != nil { log.Fatal(err) }
    defer file.Close()
    tables, err := mesher.DecodeTables[uint32](file)
    if err != nil { log.Fatal(err) }
    renderer := mesher.NewMesher(tables)
    mesh, err := renderer.MeshBlock(uint32(*id))
    if err != nil { log.Fatal(err) }
    if err := mesh.Marshal(os.Stdout); err != nil {
        log.Fatal(err)
    }
}
```

自己的数值 ID 系统可以直接构建 `mesher.Tables[B]{Index, Shapes, FullHeight}`，用 `tables.Encode(writer)` 保存。`ModelIDs(id)` 和 `Shape(modelID)` 返回共享只读数据；`FullHeight` 把原模型编号映射到满高度编号。`mesher.Shape` 是 `geometry.Shape` 的别名，便于直接组装表而不复制几何类型。自定义 64 位 ID 可写成 `type BlockID uint64`，并使用 `Tables[BlockID]`、`DecodeTables[BlockID]` 和 `[]BlockID`；ModelID 始终是 uint32 的 Shape 下标。

多个方块使用同一个 Mesher。`MeshSubchunk(blocks []B, options Options)` 接收恰好 4096 个 ID，长度不足或超出会返回错误。索引为 `(x << 8) | (z << 4) | y`，即 Y 最快、Z 次之、X 最慢，三个坐标均为 0…15。输出目标 `writer` 是任意 `io.Writer`：

```go
// blocks: []uint32，按上述顺序填满 4096 个 ID。
result, err := renderer.MeshSubchunk(blocks, mesher.Options{})
if err != nil {
    return err
}
if err := result.Mesh.Marshal(writer); err != nil {
    return err
}
```

演示侧使用 `celler/layer_storage`。已有 Palette 时，用 `CopyFrom` 展开成 Plain，再直接传入它的 `Data` 切片，其顺序与子区块入口一致：

```go
// layer: *layer_storage.Palette[uint32]
var plain layer_storage.Plain[uint32]
plain.CopyFrom(layer)
result, err := renderer.MeshSubchunk(plain.Data, mesher.Options{})
if err != nil {
    return err
}
if err := result.Mesh.Marshal(writer); err != nil {
    return err
}
```

几何传输入口在 `neomega-viewer/mesher/geometry`。`Mesh.Marshal(w io.Writer)` 写出 NVMS，`geometry.UnmarshalMesh(r io.Reader)` 读回 Mesh；文件、HTTP 响应和内存缓冲使用同一接口。`Quad.Marshal(w)` 和 `geometry.UnmarshalQuad(r)` 用于只有一个四边形的 NVMS。`Mesh.Buffers()` 保留为扁平 GPU 属性入口，单方块 JSON 和直接绘制的 Go 调用者仍使用它。

NVMS 保存 float32 绘制数据，按材质分批后的顺序恢复四边形。它不保留 CullFace，也不保留“零 ShadeNormal 表示使用 Normal”的原始标记；需要保留烘焙精度和剔面信息时，用 NVGT 保存 Tables。

构建期间调用者应保持输入切片和 Tables 不变。同一个 Mesher 可以并发处理不同输入，输出网格的修改不会改变共享表。两个 Mesh 方法只返回几何和统计，Java 状态由调用者从 `blockstate` 查询。

演示程序用 `demo/blockmesh.Open` 加载内嵌表，或通过 `OpenOptions{TablesPath: "/path/to/geometry.nvtables"}` 加载文件及配套的 `.java.json` 旁文件。`demo/singleblock` 是补齐 Binding、Renderer 与 JSON 响应的薄壳；服务和层示例直接调用 mesher。

Java 示例将完整状态文本规范化、计算 hash，再输出带 Java 元数据的 JSON。属性名和值使用 IR 的双引号语法：

```sh
go run ./demo/examples/single-block -java 'minecraft:oak_slab ["type"="bottom"]' > /tmp/slab.json
```

需要跨工具交换方块层时，`demo/subchunk.EncodeIDs` 输出恰好 **4096 个小端 uint32 ID，共 16,384 字节**。索引为 `(x << 8) | (z << 4) | y`，即 Y 最快、Z 次之、X 最慢。`demo/subchunk.ParseIDs` 将其还原成 Palette。每个 ID 都是 Java block hash。这个 `.nvblocks` 格式保存逻辑值，不依赖 Palette 的内部内存或原生 dump 布局。

例如，先运行 `go run ./demo/examples/subchunk -scene solid -blocks /tmp/solid.nvblocks`，再在网页上传 `/tmp/solid.nvblocks`，或在命令行再次读取它：

```sh
go run ./demo/examples/subchunk -input /tmp/solid.nvblocks -mesh /tmp/reloaded.nvmesh
```

## 渲染含水方块：拆层后多层渲染

几何表的键是不带 `waterlogged` 的单层状态，称为 JavaBlockHashRSL（Render Single Layer），一个 RSL 对应一组模型。完整 Java 状态的 hash 可能带 `waterlogged="true"`，它需要台阶和水两组模型，不能直接查表。

`neomega-viewer/splitlayer` 负责把完整状态拆成若干 RSL，下标即层号。规则只看状态：有 `waterlogged` 就去掉它放进第 0 层，值为 `true` 时第 1 层放一格 `minecraft:water ["level"="0"]`；seagrass、tall_seagrass、kelp、kelp_plant 总在水里，同样补一层水；其余方块原样放第 0 层。

- `splitlayer.SplitBlock(block ir.IRBlock)` 直接按规则拆一个状态，适合手里是 blockstate 的调用方。
- `splitlayer.New()` 把规则对 `presets/java/v1` 的状态全集展开成 hash 表，多个实例共享。`SplitHash(hash)` 拆一个 hash；`SplitSubchunk(blocks)` 拆 4096 格，返回 `[][]uint32`，层数取各格最大值，空位填空气。表里没有的 hash 原样当作 RSL。

拆层只能做一次：kelp 的 RSL 再拆一次会多出一层水。输入若已丢了 `waterlogged`，拆层也无从得知它原本含水。

`Mesher.MeshMultiLayerSubchunks(layerBlocks [][]B, options)` 接收多层，每层 4096 个 RSL，索引与 `MeshSubchunk` 相同。`MeshSubchunk` 就是只有一层的特例。接续前面的 `blocks` 和 `renderer`：

```go
splitter := splitlayer.New()
layers := splitter.SplitSubchunk(blocks)
result, err := renderer.MeshMultiLayerSubchunks(layers, mesher.Options{})
if err != nil {
    return err
}
if err := result.Mesh.Marshal(writer); err != nil {
    return err
}
```

演示服务的子区块接口就按这个顺序处理输入。为什么按位置合并各层、层序在同格里意味着什么、镂空前景下水面为何缩进，见 [多层渲染方案.md](多层渲染方案.md)。

## 渲染带 NBT 的方块：分发与快照

一个子区块除了方块 hash 层，还可能带 NBT：`map[[3]uint8]string`，键是格子坐标 (x, y, z)，值是 SNBT 文本。`dispatcher.Pipeline` 接收这两样，输出一个 NVFR 快照：

- **mesher 段**：静态几何。普通方块，以及尚未支持的 NBT 方块。
- **effector 段**：特效消息，例如 `{pos: [3,1,3], effect: "enchanting_table_book"}`。本体照常在 mesher 段里，特效由前端程序驱动。
- **block actor 段**：按 NBT 生成的几何，坐标已是子区块局部坐标。这些格子不再进 mesher。

组装时指定启用哪些特效和 block actor 处理器，路由表据此生成，所以没启用的方块会留在 mesher 里，不会凭空消失。下面的组装与演示服务 `demo/httpapi/frame.go` 相同，`tables` 来自几何表，`surfaces` 把贴图名查成图集里的材质：

```go
pipeline := dispatcher.NewPipeline(
    tables,
    effector.New(map[string]string{
        "minecraft:enchanting_table": "enchanting_table_book",
    }),
    blockactor.New(decoratedpot.New(tables, surfaces)),
)
result, err := pipeline.Build(blocks, nbt, dispatcher.Options{})
if err != nil {
    return err
}
if err := result.Frame.Marshal(writer); err != nil {
    return err
}
```

`Build` 内部先拆层、再分流：交给 block actor 的格子只移除本体层，水层仍交给 mesher。NVFR 的字节布局见 [merger/FORMAT.md](merger/FORMAT.md)。

新增一种 NBT 方块时，在 `blockactor/` 下新建一个子包，实现 `blockactor.Handler`（报出认领的方块名，按 `Input` 里的状态和 SNBT 返回格内几何），再在组装处传给 `blockactor.New`。陶罐的 NBT 结构与朝向见 [blockactor/decoratedpot/NBT.md](blockactor/decoratedpot/NBT.md)。

## 生成和更新几何表

模型、图集布局或静态材质策略改变后，需要重新生成配套几何。生成器枚举 Java 状态、选择模型、烘焙形状并计算边界覆盖：

```sh
go run ./scripts/gen-geometry \
  -assets ../neomega-assets/dist/final \
  -output resources/geometry.nvtables
wc -c resources/geometry.nvtables resources/geometry.nvtables.java.json ../neomega-assets/solidtable/geometry.nvsolid
```

`resources/geometry.nvtables` 是入库产物。用 Java 1.21.4 的 `../neomega-assets/dist/final` 执行上述命令，得到 18,242 个索引条目、6,414 个 Shape、19 条满高度映射，以及 12 个跳过的 `moving_piston` 状态。几何文件为 **5,796,205 字节（5.53 MiB）**。生成器同时写出 `resources/geometry.nvtables.java.json`，保存 Java 资源版本、渲染类别和跳过原因，大小为 78,357 字节。两文件合计 5,874,562 字节（5.60 MiB）。更换输入后以命令输出为准。

同一命令还生成实心表：每个成功状态的全部部件合并后，对六个方向调用 `meshing.OpaqueBoundary`，记下哪几面被不透明矩形完整覆盖。neomega-converter 的 fixer 靠它判断邻居的某一面是否实心。实心表由几何算出，所以在这里生成；但它属于共享资源，写到 `../neomega-assets/solidtable/geometry.nvsolid`（`-solid` 可改），由 `neomega-assets/solidtable` 内嵌并提供编解码。使用上述资源得到 **18,242 条、91,222 字节**。重新生成后，assets 与 converter 都需重新构建。

NVGT 保存纯数值索引与完整几何，按首次出现顺序共享材质、Vec3、Vec2，按 ID 排序写 map。向量保留 float64 位模式，颜色和材质采样区域保留 float32，动画帧数与每帧 tick 数保存为 uint32。字段偏移、宽度、引用关系和解析顺序见 [NVGT v2 格式说明](mesher/FORMAT.md)，其他语言可以直接按布局实现读取器。

演示加载器通过 `go:embed` 读取资源，与工作目录无关；`sync.OnceValues` 缓存几何表解码结果。重新生成仓库表后，重新构建演示程序即可带入新表。演示命令的 `-tables` 指定外部文件时，同时读取 `<表路径>.java.json`。直接使用 mesher 的程序只需几何文件。

几何中的 UV 区域对应生成时的图集布局。服务的 `-assets` 与 `-tables` 应来自同一套资源。更换图集后需同时更新表文件，单独改变服务的 `-assets` 不会重新烘焙。

## 运行浏览器

浏览器需要几何表与生成它时使用的图集。`neomega-blocks` 提供 Java 状态全集，`neomega-assets` 提供对应版本的模型与图集。

默认状态表由 `blocks.JavaFullStates()` 枚举，目标为 **Java 1.21.4**，共 **18,254** 个状态。该迭代器已经规范化状态，`blockstate` 将它们建成共享身份表，供单方块响应与目录使用。前端从目录取得 ID 并回传。调用者提供的状态也必须先规范化再计算哈希。属性按字典序排列，才能让同一状态的不同书写顺序得到相同的 hash。规范化由调用者安排，便于复用已规范化的状态，避免每次哈希重复排序。

需要 Go 1.25.5 或以上、Node.js 22.12 或以上，以及支持 WebGL 2 的浏览器。本地模块按相邻目录组织：

```text
new-world/
├─ neomega-assets/
├─ neomega-blocks/
├─ neomega-celler/
├─ neomega-converter/
├─ neomega-nbt/
└─ neomega-viewer/
```

如果还没有资源，在 `neomega-assets` 中生成一次：

```sh
go run ./cmd/neomega-assets build -version 1.21.4
```

生成器读取交付目录 `neomega-assets/dist/final`，并检查资源版本与 Java 状态版本是否一致。服务从 `-assets` 指定的目录读取 `blocksAtlases.json`，并提供配套的 `blocksAtlasLatest.png` 和 `THIRD_PARTY_NOTICES.txt`。

子区块页面启动时还会加载附魔书贴图。服务从相对于 `-assets` 的 `../works/04-renderers/assets/minecraft/textures/entity/enchanting_table_book.png` 读取它。使用默认资源目录时，这个文件位于 `neomega-assets/dist/works/04-renderers/assets/minecraft/textures/entity/enchanting_table_book.png`。部署资源时需要一并保留这条相对路径。资源更新方式见 [neomega-assets README](../neomega-assets/README.md)。

然后在 `neomega-viewer` 中构建网页并启动服务：

```sh
npm --prefix demo/web ci
npm --prefix demo/web run build
go run ./demo/cmd/neomega-viewer
```

两个入口有不同的观察目的：

| 页面 | 用途 |
|---|---|
| <http://127.0.0.1:2410> | 搜索方块、选择状态，检查完整状态对应的外形 |
| <http://127.0.0.1:2410/subchunk.html> | 观察多个形状的交界，开关剔除与合并，上传方块层、下载二进制网格 |

子区块页提供石头、玻璃、半砖、草木和混合场景。勾选“三角网格”后，可以直接比较合并前后的拓扑。

资源或网页位于其他地方时，显式传入路径：

```sh
go run ./demo/cmd/neomega-viewer \
  -assets /path/to/dist/final \
  -tables /path/to/geometry.nvtables \
  -web /path/to/web/dist \
  -listen 127.0.0.1:2410
```

开发网页时，保持 Go 服务运行，在另一终端执行 `npm --prefix demo/web run dev`，然后打开 <http://127.0.0.1:2411> 或其 `/subchunk.html` 页面。Vite 将 API、图集和资源声明请求代理到 Go 服务。

## 一个方块怎样变成几何

`scripts/internal/tablegen.Builder` 组合下面的生成步骤，使用模型继承缓存和材质库。`model`、`bake` 和 `materials` 均指 `scripts/internal/` 下的生成包。模型通过 UV（纹理上的二维坐标）指定每个表面点采样的位置：

```text
Java 状态全集
  │ blockstate：枚举名称、属性与规范化 hash
  ▼
完整 Java 状态
  │ model.SelectFirst：选择 variant，收集命中的 multipart 部件
  ▼
模型引用 + 旋转 + UV 锁定
  │ model.Resolver：解析父模型、合并纹理槽、跟随 #纹理别名
  ▼
模型元素与面
  │ bake.Bake + materials.Library
  ▼
局部四边形 → 去重为 Shape，写入两张表
```

`variant` 按状态选择候选模型。`multipart` 是按条件叠加多个模型部件的规则。建表使用 `SelectFirst` 固定取首项，并收集所有命中的部件。完整 Java 状态已经包含连接方向、半砖位置等选择条件，所以模型选择在生成时就能确定。运行时按状态 hash 直接取模型列表，属性变化则使用另一个状态的 hash。

索引表与几何表分开，是为了让不同状态共享相同部件。例如栅栏柱出现在多种连接组合里，若每个状态都保存完整几何，就会重复存储同一根柱子。两张表让各状态只保存部件编号，共用的 Shape 保存一份。父模型解析也按模型 ID 缓存。

运行时两个入口都先调用 `Tables.ModelIDs(javaHash)`，再用 `Tables.Shape(modelID)` 取得部件。单方块将部件四边形复制到独立网格，子区块保留共享引用供邻域剔面使用。网页属性选项来自 catalog 的同名状态分组；每次修改属性都会选择另一个完整状态，并把它的 hash 发给 `/api/block`。

几何使用一个方块为一单位的局部坐标，Y 向上，北向为 −Z。四边形保存几何法线、UV、颜色和材质。模型要求覆盖明暗方向时，另外保存光照法线，真实几何法线仍保持原义。`CullFace` 保存模型允许被邻居遮挡的方向；单方块入口保留这些面，子区块入口检查其实际覆盖情况。

## 形状、透明度与静态外观

半砖的半高来自模型几何，草的外形来自旋转的交叉平面。几何形状和贴图透明度需要分别处理：

| 表面类型 | 示例 | 绘制方式 |
|---|---|---|
| 不透明 `opaque` | 石头、半砖、栅栏 | 写入深度，同材质面合批 |
| 镂空 `cutout` | 树叶、草 | 丢弃透明像素，其余像素写入深度 |
| 半透明 `blend` | 玻璃、水 | 混合颜色，不写入深度，逐面参与远近排序 |

`scripts/internal/materials.Library` 根据图集像素分类。`scripts/internal/resources.PreviewPolicy` 补充玻璃等语义规则，以及草木、水和红石颜色。红石颜色在建表时按 `power` 烘入 `Quad.Color`。形状去重比较完整四边形，包括颜色。有厚度的镂空元素使用正面绘制，避免多画盒体的背面。独立薄片可以双面显示；草已有明确前后面时，分别绘制对应一侧，避免重复绘制。前端在每张纹理自己的图集区域内采样；即使一个面合并到跨越多个方块，也会重复纹理。

箱子、床和告示牌等方块的完整外观由游戏代码绘制，JAR 未必包含完整模型。`neomega-assets` 的 `blockRenderers.json` 说明类别并补入社区静态模板，viewer 通过同一模型接口解析它们。图集 PNG 单独加载，不随每个网格重复传输。

这里的静态指**给定完整状态后，模型姿态固定**。箱子使用固定姿态，告示牌展示牌体。水、岩浆、火等纹理由浏览器按图集帧信息循环播放，几何保持不变。库不接收方块实体的实时外观或动画状态。子区块中的同类流体上方还有流体时，补满当前格的液面高度。这不是游戏的流体曲面模拟。

动画纹理的全部帧在图集中竖排，Surface 的 Region 覆盖整条区域。生成器直接搬运图集的 Frames 和 FrameTime；后者以 tick 为单位，每 tick 为 1/20 秒。shader 先按时间选择一帧，再在这一帧内重复局部 UV，并按单帧尺寸内缩半像素以免采到相邻帧。静态纹理使用 Frames=1、FrameTime=0。

状态全集暂不包含 `waterlogged`，这是涉及前景、背景方块表示的历史约定。当模型条件需要它时，建表补为 `false`。网页仅提供状态全集中已有的属性选项。`air` 等不可见类别返回空几何。`moving_piston` 缺少决定移动内容的数据，会返回无法确定静态外形的错误。

水和岩浆的状态包含 `level=0…15`，资源的空 variant 不按 level 选择模型。流体预览使用 level 决定高度。建表时另存满高度几何，并建立原模型编号到满高度编号的映射。子区块只比较上方流体的数值消面组，再切换到满高度形状。

## 多个方块放在一起时

直接叠加 4096 份方块网格，会保留大量完全看不见的内面。子区块入口遍历输入切片的全部 4096 个位置，再查询各格的共享 Shape 引用。演示侧的 Palette 用调色板索引压缩方块 ID，调用前用 `CopyFrom` 展开成 Plain，再传入 `plain.Data`。默认填满的格子同样参与渲染。

栅栏、玻璃板和墙的连接方向已经包含在输入 ID 对应的 Java 状态中。调用者写入确定的连接状态，渲染时直接查表取得已经选好的部件。

若输入的栅栏、玻璃板或铁栏杆连接属性不可靠，调用者应先用 neomega-converter 的 fixer 修复，再把结果交给渲染入口。

多层输入时，各层的几何按位置并进同一格，再一起剔面与合并，所以含水台阶的水和旁边的普通水之间照常剔掉内面。同一格内，前层的不透明面会裁掉后层与它共面的部分；前层是树叶这类满格镂空方块时，后层改为缩进到 0.99 倍并跳过剔面，避免共面闪烁。上方格任意一层有同类流体，本格流体就切换成满高度。

`prepareNeighborhood` 准备各格的共享几何引用，并根据上方邻居切换流体高度。独立子区块的外部按空覆盖处理。剔面和合并放在同一个渲染入口内，因为剔面需要直接访问邻居几何，并可能留下半个面这样的局部片段。这样可以直接把剩余面片交给合并步骤，省去另行传递一份裁剪结果。确定形状后，`meshing` 分两步减少面：

1. **剔除实际被盖住的部分。** 模型声明可剔除、且位于格子边界的轴对齐矩形，才会检查邻居。邻居覆盖直接来自 Shape.Boundary，无须逐格重扫几何。完整不透明面可以遮挡；多个部件的矩形并集也可以形成覆盖。石头紧贴底半砖时，只裁掉下半部分，并为保留部分重新插值 UV。斜面和非矩形保守保留。
2. **合并相邻矩形。** 在同一平面上按实际矩形边界建立二维网格，贪婪地扩展连续区域。只有材质、颜色、法线、光照方向、UV 变化率和重复相位兼容时才合并。合并后的 UV 延伸到多个纹理周期，保持每个方块原有的纹理尺寸。

透明面有额外规则。建表时将玻璃名称和流体家族转成 Shape 的数值消面组，避免渲染时解析名称。模型的 CullFace 也在烘焙时转为数值方向。相同玻璃、同类流体可以移除实际重叠的内面；不同玻璃保留界面。玻璃和树叶不会作为不透明遮挡物盖掉后面的石头。半透明面不参与大矩形合并，前端仍可按四边形排序。逐面排序不保证任意相交半透明表面的严格顺序。

所有格子复用建表时固定的模型候选。即使名称相同，UV 不兼容的面也不会强行合并。剔面与摆放由 `cullAndPlace` 处理，合并由 `merge` 处理，得到 `geometry.Mesh`。传输时调用 `Mesh.Marshal()`；需要扁平属性或 JSON 时调用 `Mesh.Buffers()`。

独立贪婪矩形可能形成 T 形接缝，即较短边接在另一条长边的中间；算法不额外缝合这些边。外边界按暴露面处理；组装地图时还需要提供相邻子区块的边界信息。子区块解析不创建地图或 worker，地图加载与卸载属于另一个入口的调度职责。

## 几何怎样传给绘制端

单方块的 JSON 便于查看状态和几何。子区块则有更多顶点，数值文本解析和临时数组会产生额外开销。因此子区块通过 `Mesh.Marshal` 输出 **NVMS v1 二进制**：材质表使用一小段 JSON，顶点属性和索引使用对齐的小端数值数组。

浏览器的 `decodeMesh` 在常见的小端平台上为响应 `ArrayBuffer` 建立 `Float32Array`、`Uint32Array` 视图。Three.js 适配层直接保留这些视图；GPU 上传仍由 Three.js 完成。协议不依赖 HTTP，同一份数据也可以写入 `.nvmesh` 文件。布局与解析接口见 [NVMS v1 格式说明](mesher/geometry/FORMAT.md)。

网格合并与编码解决的是不同问题。以 Java 1.21.4 资源、当前 Java 状态全集、固定首候选的满石头层为例：

| 开启的处理 | 输出四边形 | NVMS 字节数 |
|---|---:|---:|
| 均关闭 | 24,576 | 6,095,076 |
| 仅面剔除 | 1,536 | 381,156 |
| 面剔除与合并 | 6 | 1,716 |

这些是未压缩的文件大小。下面的命令在仓库根目录生成表中三份文件，并输出各阶段面数：

```sh
go run ./demo/examples/subchunk -scene solid -cull=false -greedy=false -mesh /tmp/solid-raw.nvmesh
go run ./demo/examples/subchunk -scene solid -greedy=false -mesh /tmp/solid-culled.nvmesh
go run ./demo/examples/subchunk -scene solid -mesh /tmp/solid.nvmesh -blocks /tmp/solid.nvblocks
wc -c /tmp/solid-raw.nvmesh /tmp/solid-culled.nvmesh /tmp/solid.nvmesh
```

更换资源、模型选择或编码版本后，应重跑命令核对数量。

## 复用前端绘制

渲染核心的源码在仓库根的 `web/render/`，和 Go 侧的 `mesher/` 对称；`demo/web` 只放演示网页。外部页面需要自行管理网络和场景时，可使用 `demo/web` 的 `@neomega/viewer/render` 导出，它转导出 `web/render/`，只依赖 Three.js，不依赖演示页、方块目录、地图或 worker：

```sh
npm install /path/to/neomega-viewer/demo/web
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

// 在宿主已有的每帧回调中，先更新时间，再绘制场景。
function render(timestamp: number): void {
  mesh.updateTime(timestamp / 1000);
  renderer.render(scene, camera);
}

// 不再使用时释放本对象的缓冲与材质；共享图集由调用者管理。
scene.remove(mesh.object);
mesh.dispose();
atlas.dispose();
```

`createMeshObject` 也接受单方块 JSON 的数值数组。宿主在 `requestAnimationFrame` 回调中调用 `updateTime(timestamp / 1000)`，让同一网格的材质共享秒数；`animated` 表示是否需要持续绘制。希望一并复用相机、视口和动画驱动时，使用 `MeshViewport`，再调用 `set(data)`；`BlockViewport` 保留为原有名称的兼容导出。视口只在存在动画材质、特效或相机仍在移动时连续请求帧。

NVFR 快照用 `decodeFrame` 解码，三段画进同一个场景，共享深度和透明排序。特效由注册表把特效名映射到工厂，`createEffectRegistry` 内置附魔书，书的贴图由调用方加载并持有：

```ts
import { MeshViewport, createEffectRegistry, decodeFrame, loadAtlas } from "@neomega/viewer/render";

const [atlas, book] = await Promise.all([
  loadAtlas("/assets/atlas.png"),
  loadAtlas("/assets/entity/enchanting_table_book.png"),
]);
const viewport = new MeshViewport(host, atlas, { effects: createEffectRegistry(book) });
const response = await fetch("/api/subchunks/block_entities/frame");
viewport.setFrame(decodeFrame(await response.arrayBuffer()));
```

换一个快照会整体替换三段内容，空列表表示清空。不在 Three.js 视口里使用时，`createFrameObject(data, atlas, registry)` 返回一个可放进自己场景的对象。注册其他特效时，往 `createEffectRegistry` 返回的 Map 里添加工厂即可。

## 接口与扩展位置

仓库分发的 Java 几何表使用 `blockstate.Java()` 枚举的状态集，其状态不带 `waterlogged`。单方块身份查询与几何接口接受该状态集中的 hash。子区块入口接受完整 Java hash，包括含水变体，经拆层得到 RSL 后查询几何表。mesher 本身可以使用调用者提供的其他数值 ID 与表。需要改变固定颜色和材质语义时，在生成侧通过 `tablegen.OpenOptions.Policy` 提供 `materials.Policy`，并重新生成表。演示加载器的 `OpenOptions` 选择 `TablesPath`。

其他变化可以按职责定位：

| 要解决的问题 | 模块 |
|---|---|
| ID 对应哪个 Java 状态？ | `blockstate` |
| 如何修复转换丢失的连接、拐角与门状态？ | neomega-converter 的 `fixer`；实心表在 `neomega-assets/solidtable` |
| 含水方块怎样拆成多层？ | `splitlayer` |
| 带 NBT 的方块、特效怎样分流和打包？ | `dispatcher`、`effector`、`blockactor`、`merger` |
| 状态命中哪些部件，模型怎样继承？ | `scripts/internal/model` |
| 一个状态怎样烘焙成局部几何？ | `scripts/internal/tablegen`、`bake`、`materials`、`resources` |
| 怎样编码与查询共享几何？ | `mesher.Tables[B]`；演示文件加载在 `demo/blockmesh` |
| 如何构建单个方块？ | `Mesher.MeshBlock`；Java 响应薄壳在 `demo/singleblock` |
| 如何从 4096 个 ID 准备邻域几何？ | `Mesher.MeshSubchunk`；多层用 `Mesher.MeshMultiLayerSubchunks` |
| 哪些表面被覆盖，哪些矩形能够合并？ | `mesher/meshing` |
| 几何怎样编码和解码？ | `mesher/geometry` 的 `Mesh.Marshal`、`UnmarshalMesh`；前端 `web/render/binary.ts` |
| 哪些代码只是演示？ | `demo/` 下的服务、目录、场景和网页 |

演示服务提供以下接口：

| 请求 | 返回内容 |
|---|---|
| `GET /api/blocks` | JSON 名称分组、示例 ID、版本和图集地址 |
| `GET /api/states?name=minecraft:oak_fence` | JSON 状态列表与真实 ID |
| `GET /api/state?id=<uint32>` | JSON Java hash 对应的状态 |
| `GET /api/block?id=<uint32>` 或 `POST /api/block` | JSON 单方块状态、渲染类别和几何 |
| `GET /api/subchunks` | JSON 子区块示例目录 |
| `GET /api/subchunks/{scene}/mesh` | NVMS 二进制几何 |
| `GET /api/subchunks/{scene}/blocks` | `.nvblocks` 逻辑方块层 |
| `POST /api/subchunk` | 提交 `.nvblocks` 原始字节，返回 NVMS |
| `GET /api/subchunks/{scene}/frame` | NVFR 快照：静态几何、特效、block actor |
| `POST /api/subchunk/frame` | 提交 `.nvblocks` 原始字节（无 NBT），返回 NVFR |
| `GET /assets/atlas.png` | 与材质区域配套的 PNG |
| `GET /assets/entity/enchanting_table_book.png` | 附魔书特效的贴图 |

`POST /api/block` 的正文为 `{"id": <javaHash>}`。返回的 `binding` 标识完整 Java 状态，`renderer` 描述静态类别，`geometry` 包含绘制数据。

子区块网格接口接受 `cull`、`greedy` 布尔查询参数，两项默认开启。统计值通过 `X-Neomega-Mesh-Stats` 响应头返回。`shapes` 表示实际引用的共享 Shape 数量，空气没有 Shape 引用。完全移除的源面与局部裁切的源面分别计数。局部裁切可能产生多个保留片段，因此“原始面减去移除面”不一定等于“剔除后面数”。

上传已有方块层时，直接发送二进制正文：

```sh
curl -fsS http://127.0.0.1:2410/api/subchunk \
  -H 'Content-Type: application/octet-stream' \
  --data-binary @/tmp/solid.nvblocks \
  -o /tmp/uploaded.nvmesh
```

输入格式或参数无效返回 400；无法确定静态几何返回 422，错误正文使用 JSON。未知示例返回 404。

## 验证解析与传输

单元测试覆盖 uint32/uint64 及自定义类型、独立二进制布局样本，并检查模型继承、条件选择、旋转、UV、覆盖并集与孔洞、局部裁切、贪婪兼容条件和二进制布局：

```sh
go build ./...
go test ./...
go vet ./...
go list -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./mesher/...
npm --prefix demo/web test
npm --prefix demo/web run build
```

依赖命令只列出非标准库包，结果只有 `neomega-viewer/mesher` 及其三个子包。NVMS 测试使用固定样本逐字节比较输出，并检查 io 错误传播。样本保留旧编码器的几何字节，材质 JSON 包含动画帧信息。

默认测试使用内嵌表，覆盖完整状态集、全部演示场景、红石颜色和流体高度。加入真实资源后，测试会重新建表，逐项比较全部 ModelIDs、Shape 四边形、Boundary 和 FullHeight，并验证 NVGT 往返、入库文件与内嵌表一致。HTTP 测试同时检查图集、单方块响应、子区块上传和二进制输出：

```sh
NEOMEGA_ASSETS_DIR="$PWD/../neomega-assets/dist/final" \
  go test -race -v ./... -count=1 -timeout=90s
```

Java 1.21.4 状态全集的单方块全表检查生成 18,242 个静态结果，另有 12 个 `moving_piston` 状态明确返回静态能力边界。实际数量以更换数据后重新运行的输出为准。

要检查 Go 编码器与 TypeScript 解码器的配合，可使用前面生成的满石头网格：

```sh
NEOMEGA_MESH_FIXTURE=/tmp/solid.nvmesh npm --prefix demo/web test
```

该检查验证面数、范围、重复 UV，以及解码器和 Three.js 适配层对原始数值缓冲的共享。

## 来源与许可

原创代码采用 [MIT](LICENSE)，Three.js 使用 MIT。Palette 来自相邻的 `neomega-celler`。模型和纹理由 `neomega-assets` 提供，包含 Minecraft 资源和 `zardoy/mc-assets` 社区模板；生成网格不会改变素材权利来源。部署时应保留交付目录中的 `THIRD_PARTY_NOTICES.txt`。转换数据有独立许可，不能由本项目 MIT 覆盖。详见 [NOTICE.md](NOTICE.md)。
