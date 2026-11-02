# neomega-viewer

本模块位于工作区的 `apps/` 能力组装层，Go 模块名保持为 `neomega-viewer`。

本模块组合存档读写、转换修复和渲染核心，提供 Java／Bedrock 存档浏览器与方块演示应用。
Go 服务按视点读取与缓存数据，浏览器负责交互、加载和页面展示。
具体网格生成及 Web 绘制接口来自 [neomega-viewer-core](../../neomega-viewer-core/README.md)。

`cmd/world-viewer` 负责存档打开、切换与前端页面，`source` 适配不同数据来源，`column` 编码区块列，`viewhttp` 管理 HTTP 调度与缓存。
`demo` 提供方块选择、场景检查和上传预览。
最简核心示例位于 [viewer-core/viewer/examples](../../neomega-viewer-core/viewer/examples)。

## 浏览 Java 与 Bedrock 存档

在本模块目录构建程序，再指定存档目录启动。网页、贴图和几何表均已内嵌：

```sh
go build -o cmd/world-viewer/world-viewer ./cmd/world-viewer
./cmd/world-viewer/world-viewer -world /path/to/java-world
```

打开 <http://127.0.0.1:2412>。页面是全屏 Three.js 场景，左上角显示坐标轴、坐标、帧率和网格内存估算，右下角的可收起面板提供可视化打开/切换存档、坐标跳转、线框、视距和渲染分辨率。省略存档参数时显示空场景。`-world` 同样接受包含 `db/` 的 Bedrock 世界目录，`level.dat` 可以缺失。读取 Java PackFS 存档时改用 `-pack /path/to/world.pack`。

Bedrock 方块在内存中转换为 Java 状态，默认经过邻接修复，再进入相同的渲染路径。转换和修复结果不写回存档。`-bedrock-fix=false` 可以只看转换结果；`-dimension` 对 Bedrock 使用编号 0、1、2，默认 0。

程序只读访问相机附近的区块，重叠 section 保持原网格。视距默认 8、最多 30 个区块。前端源码独立放在 `cmd/world-viewer/frontend`，构建后的程序可以单文件分发。资源准备、维度选择、键鼠操作与内存统计见 [存档浏览说明](cmd/world-viewer/README.md)。

## 用 section 来源组装浏览服务

浏览存档、结构文件和内存场景时，变化的是方块从哪里来，渲染和网页加载的过程可以相同。一个 section 是 16×16×16 的方块区域。来源实现 `dispatcher.SectionSource`，按 section 坐标返回完整 Java 状态调色板和格内方块实体；不存在的位置返回 `nil, nil`，表示空气。这样，每种输入格式只需适配来源，浏览服务可以继续使用同一套组件。

从数据到页面分成三步：来源提供 section，`dispatcher.Pipeline.BuildSections` 读取中心与六向邻居并生成快照，`column.BuildColumn` 将一列快照编码为 NVWC 字节。NVWC 是网页加载一段竖直区块列时使用的响应格式。`viewhttp` 调度构建、缓存这些字节并提供 HTTP 路由。它接收一个构建函数，因此来源与 Pipeline 由程序自行组合。

| 组件 | 负责的边界 |
|---|---|
| `source.MemorySections` | 用 map 提供只读 section，适合手搓场景或已载入内存的结构 |
| `source/javaworld.Source` | 独占已打开的 Java 世界，缓存原始记录；每次 `Reader()` 独立解码 |
| `source/bedrockworld.Source` | 接收 `adapt.ChunkSource`，借用转换柱子的调色板；每次 `Reader()` 保存请求内的柱子引用 |
| `column.BuildColumn` | 构建请求高度范围，汇合渲染与来源诊断，编码 NVWC |
| `viewhttp.View` | 一代固定内容的 revision、取消范围、构建闭包和列字节缓存 |
| `viewhttp.Handler` | 跨代共用的 worker 限额、排队、`/api/chunk`、资源与静态页面 |
| `viewhttp.Description` | 配套页面的启动描述；固定内容可用 `DescriptionHandler` 返回它 |
| `viewassets` | 以 `fs.FS` 提供应用网页；贴图由 viewer-core 的 resources 包提供，调用方也可以换成自己的文件系统 |

### 三种来源的组装

几种来源共用渲染 Pipeline。几何表来自 viewer-core 的 resources 包，图集元数据来自与它匹配的资源。下列片段省略 import 和错误处理，完整入口见 [world-viewer](cmd/world-viewer/main.go)。

```go
resources, err := preset.Open(preset.OpenOptions{})
// 检查 err 后创建 Pipeline。atlas 是已解析的图集；nil 表示不开启陶罐 actor。
pipeline := dispatcher.NewJavaPipeline(resources.Tables, atlas, nil)
```

Java 存档需要先打开世界并选好维度。路径识别、默认维度、出生点和初始落点由 `cmd/world-viewer` 完成。`javaworld.New` 接收已经准备好的句柄，之后调用方把该句柄的独占访问权交给来源：

```go
world, err := java.OpenDirectory(directory, java.Options{
    ReadOnly:       true,
    MaxOpenRegions: 16,
})
// 检查 err；dim 是明确选择的维度，例如 java.Overworld。
src := javaworld.New(world, dim)
view := viewhttp.NewView(revision, func(ctx context.Context, key column.Key) ([]byte, error) {
    return column.BuildColumn(ctx, pipeline, src.Reader(), key)
})
```

`Reader()` 必须放在构建闭包内部。一个 reader 缓存本次请求解码过的区块及整列诊断，由该请求串行使用。不同 worker 只共享不可变的原始记录字节。`java.World` 的内部 region 缓存会在读取时变化，因此来源用一个 goroutine 串行访问句柄。缓存中的内容以打开时为准；重新读取修改后的存档需要创建新的来源。

Bedrock 需要先把状态转换为 Java 表达。`neomega-converter-core/adapt` 的 `Converted` 按坐标读取、转换并缓存柱子，`neomega-converter/adapt.Fixed` 在它外面叠加可选的邻接修复。两层都实现 `adapt.ChunkSource`，因此 `bedrockworld.Source` 只依赖这个取柱接口。选择转换方式属于组装方，渲染来源只负责借用调色板、分组方块实体和转接诊断。

以下 `adapt` 来自 converter-core，`fixedadapt` 来自 converter；`renderresources` 来自 viewer-core/viewer/resources。

下面的 `read` 是组装方提供的 `adapt.ReadChunk`，绑定已经只读打开的 `bedrock.World` 和维度编号。它在访问句柄前登记在途读取，并在返回时注销。完整的打开与等待实现见 [bedrock.go](cmd/world-viewer/bedrock.go)。

```go
converted := adapt.NewConverted(read, adapt.Options{})
var chunks adapt.ChunkSource = converted // 只转换。
if repair {
    // f 由 fixer.New() 创建一次，可以供多个存档的修复层共享。
    chunks = fixedadapt.NewFixed(converted, f, adapt.Options{})
}
src := bedrockworld.New(chunks)
view := viewhttp.NewView(revision, func(ctx context.Context, key column.Key) ([]byte, error) {
    return column.BuildColumn(ctx, pipeline, src.Reader(), key)
})
```

每次构建也使用独立 reader，保留本次取得的共享只读柱子引用。转换和修复缓存由 adapt 管理，同一来源的不同 reader 可以复用它们。未知方块在转换时已变成空气，其按状态汇总的诊断仍随请求柱子返回。展示框等普通实体保存在 `ConvertedChunk.Entities` 中，当前 dispatcher 只接收方块及方块实体，因此这些实体不会显示。

内存场景直接填 map，无须构造世界句柄。下面在 section 原点附近放一块石头；map 的键是 section 坐标，`layer_storage.At` 使用 section 内的方块坐标：

```go
blocks := layer_storage.MakePaletteStorage(ir.IRBlock("minecraft:air []"))
blocks.Set(layer_storage.At(1, 0, 1), ir.IRBlock("minecraft:stone []"))
sections := source.MemorySections{
    dispatcher.SectionPos{}: &dispatcher.Section{Blocks: blocks},
}
view := viewhttp.NewView(revision, func(ctx context.Context, key column.Key) ([]byte, error) {
    return column.BuildColumn(ctx, pipeline, sections, key)
})
```

交给 View 后，map、section、调色板和实体 NBT 都保持只读。修改场景时创建新的数据和 View。Pipeline 接收完整状态，栅栏连接等属性由来源准备；含水拆层由 Pipeline 处理。

### 接上页面与切换内容

有了 View，就可以接入 HTTP。`current` 每次返回当前的非 nil View；固定场景直接返回同一个指针。下面的 `mux` 是应用自己的 `http.ServeMux`：

```go
mux.Handle("/", viewhttp.Handler(
    func() *viewhttp.View { return view },
    viewhttp.Options{
        Assets:  renderresources.Resources(),
        Web:     viewassets.Web(),
        Workers: 4,
    },
))
```

配套页面启动时需要知道内容编号、名称和初始视点，应用通过 `GET /api/world` 提供这些信息。`viewhttp.Description` 定义这份场景描述，路由由应用注册。固定场景可以直接返回描述，默认的 `canOpen: false` 会隐藏打开按钮：

```go
mux.Handle("GET /api/world", viewhttp.DescriptionHandler(viewhttp.Description{
    Revision:        revision,
    Loaded:          true,
    Name:            "内存场景",
    Position:        [3]float64{16, 14, 28},
    ResourceVersion: resources.Metadata.Version,
}))
```

`world-viewer` 的 `Info` 嵌入同一个描述类型，再补充路径、存档格式和维度。它设置 `CanOpen: true`，由程序提供打开存档的 POST 和文件浏览接口。

需要切换时，程序把描述、View 和来源关闭函数放在同一个对象里原子替换。revision 是该代内容的编号，每次发布都分配新值。打开失败保留原对象。成功切换的顺序如下，具体实现在 [worlds.go](cmd/world-viewer/worlds.go)：

```go
current.Store(next)     // next 已经包含描述、新 View 和来源关闭函数。
previous.View.Retire()  // 拒绝新任务、取消旧任务，并等待在途构建结束。
previous.close()       // 等底层读取结束，再关闭 World 和文件容器。
```

Handler 每个列请求只读取一次 `current()`，避免中心和邻居来自不同内容。过期 revision 或退役期间尚未发送的响应返回 409。队列和 worker 限额属于 Handler，切换 View 不会创建额外 worker。响应开始发送时已释放构建名额，只持有生成的字节，因此慢客户端不会拖住来源关闭。已经开始发送的响应不能改成 409，配套前端切换时会丢弃旧代结果。

`Retire` 等待构建回调结束；Java 的底层 I/O 可能仍在完成，`javaworld.Source.Close()` 会进一步等待文件 goroutine 退出。这个方法归还 World 的访问权，World 和 PackFS 由打开它们的程序关闭。

Bedrock 的 adapt 等待者也可能先于底层读取返回取消。组装方关闭来源时，先禁止新的读取回调进入，再等待已登记的读取结束，最后关闭 LevelDB。只等待 `View.Retire()` 不足以归还句柄。转换和修复处理的是已经复制出的记录，原始读取结束后，它们可以独立完成；具体取消约定见 [adapt 说明](../neomega-converter/README.md#adapt按坐标读取与缓存)。

### 来源诊断与实体适配

来源读取时可能发现无法转换的状态或异常实体。来源可以额外实现 `source.ColumnDiagnostics`，在 section 读取完成后按 `celler.ChunkPos` 返回 `[]source.Issue`。`Position == nil` 表示整根柱子的汇总，非 nil 表示世界方块坐标。`Count` 保存涉及的数量，单项问题填 1；状态、实体 ID 和详情按需要提供。

`column.BuildColumn` 只查询请求柱子的诊断，保留来源返回的每一条。来源诊断覆盖整根柱子，不按请求的高度窗口裁剪，也不混入为剔面读取的邻居诊断。来源应让同一代内容的条目及计数保持一致。网页按柱子坐标和条目内容消除高度窗口之间的重复报告；同一响应中的相同条目仍分别保留。

渲染诊断按完整状态与原因去重。响应中的 `scope` 区分单格与整列，`origin` 区分渲染诊断与来源诊断，以便采用各自的去重规则。NVWC 头里的 `unknown` 只统计缺失模型的方块数量。协议布局和 JSON 示例见[读取与传输](cmd/world-viewer/README.md#读取与传输)。

Bedrock 来源保留 converter 按状态汇总的 `unknown_block` 和按实体 ID 汇总的 `unsupported_block_entity` 计数。单项 NBT 或规则错误按一项报告；有具体坐标时保留坐标。这些来源诊断始终覆盖请求柱子的全部高度，不增加缺失模型数量。

需要把读取或转换得到的实体交给 Pipeline 时，调用 `source.GroupBlockEntities(entities)`。它接收同一柱子的 `[]java.BlockEntity`，按 section Y 和格内坐标分组，支持负坐标，并复用输入 NBT。其他来源只要产出同一种实体值，就能使用这个函数。结构文件适配器可以先将结构放到所需的 section 坐标，再提供 `SectionSource`，或直接填充 `MemorySections`。

## 运行浏览器

浏览器需要几何表与生成它时使用的图集。`neomega-blocks` 提供 Java 状态全集，`viewer-assets` 提供对应版本的模型与图集。

默认状态表由 `blocks.JavaFullStates()` 枚举，目标为 **Java 1.21.4**，共 **18,254** 个状态。该迭代器返回 tidy IR 方块，目录按名称分组，身份反查直接使用 `HashToBlock`。前端从目录取得 ID 并回传。调用者提供的状态也必须先规范化再计算哈希。属性按字典序排列，才能让同一状态的不同书写顺序得到相同的 hash。规范化由调用者安排，便于复用已规范化的状态，避免每次哈希重复排序。

存档中的旧状态另由 blocks 的 Java 兼容映射覆盖。生成器按源版本从高到低取同一状态的首次解释，让旧状态 hash 共享 1.21.4 目标状态的几何；例如旧 `grass` 使用 `short_grass` 的模型。运行时不需要源版本，原始 blockstate 保留用于拆层与诊断。映射、兼容状态枚举及重建命令见 [blocks 生成说明](../../neomega-blocks/third_party/gen_resource/README.md#java-存档的兼容状态)。

需要 Go 1.25.5 或以上、Node.js 22.12 或以上，以及支持 WebGL 2 的浏览器。本地模块按以下目录组织：

```text
new-world/
├─ neomega-viewer-core/
├─ neomega-converter-core/
├─ neomega-blocks/
├─ neomega-celler/
├─ neomega-nbt/
├─ neomega-packfs/
└─ apps/
   ├─ neomega-converter/
   └─ neomega-viewer/
```

如果还没有资源，在 `neomega-viewer-core/viewer-assets` 中生成一次：

```sh
go run ./cmd/neomega-assets build -version 1.21.4
```

viewer-core 的建表器直接读取资源生成目录，并检查资源版本与 Java 状态版本是否一致。服务使用 viewer-core 的图集定义，从 `-assets` 指定的目录读取以完整纹理资源 ID 为键的 `blocksAtlases.json`，并提供配套的 `blocksAtlasLatest.png` 和 `THIRD_PARTY_NOTICES.txt`。

子区块页面启动时还会加载附魔书贴图。服务从相对于 `-assets` 的 `../works/04-renderers/assets/minecraft/textures/entity/enchanting_table_book.png` 读取它。使用默认资源目录时，这个文件位于 `neomega-viewer-core/viewer-assets/dist/works/04-renderers/assets/minecraft/textures/entity/enchanting_table_book.png`。部署资源时需要一并保留这条相对路径。资源更新方式见 [viewer-assets README](../../neomega-viewer-core/viewer-assets/README.md)。

演示网页通过 `file:../../../../neomega-viewer-core/viewer/web` 依赖仓库内的共享前端包。`npm ci` 按锁文件安装演示页依赖，并链接共享包。在 `apps/neomega-viewer` 中构建网页并启动服务：

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

开发网页时，先用上述 `npm --prefix demo/web ci` 安装依赖。保持 Go 服务运行，在另一终端执行 `npm --prefix demo/web run dev`，然后打开 <http://127.0.0.1:2411> 或其 `/subchunk.html` 页面。Vite 将 API、图集和资源声明请求代理到 Go 服务。

## 方块层交换与统计

Java 示例将完整状态文本规范化、计算 hash，再输出 IR 状态与绘制缓冲的 JSON。属性名和值使用 IR 的双引号语法：

```sh
go run ./demo/examples/single-block -java 'minecraft:oak_slab ["type"="bottom"]' > /tmp/slab.json
```

需要跨工具交换方块层时，`demo/subchunk.EncodeIDs` 输出恰好 **4096 个小端 uint32 ID，共 16,384 字节**。索引为 `(x << 8) | (z << 4) | y`，即 Y 最快、Z 次之、X 最慢。`demo/subchunk.ParseIDs` 将其还原成 Palette。每个 ID 都是 Java block hash。这个 `.nvblocks` 格式保存逻辑值，不依赖 Palette 的内部内存或原生 dump 布局。

例如，先运行 `go run ./demo/examples/subchunk -scene solid -blocks /tmp/solid.nvblocks`，再在网页上传 `/tmp/solid.nvblocks`，或在命令行再次读取它：

```sh
go run ./demo/examples/subchunk -input /tmp/solid.nvblocks -mesh /tmp/reloaded.nvmesh
```

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

## 演示接口

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

`POST /api/block` 的正文为 `{"id": <javaHash>}`。返回的 `binding` 标识完整 Java 状态，`renderer` 描述静态类别，`geometry` 包含绘制数据。`binding` 是 HTTP 展示格式，Go 核心与目录内部使用 IRBlock。

子区块网格接口接受 `cull`、`greedy` 布尔查询参数，两项默认开启。统计值通过 `X-Neomega-Mesh-Stats` 响应头返回。`shapes` 表示实际引用的共享 Shape 数量，空气没有 Shape 引用。完全移除的源面与局部裁切的源面分别计数。局部裁切可能产生多个保留片段，因此“原始面减去移除面”不一定等于“剔除后面数”。

上传已有方块层时，直接发送二进制正文：

```sh
curl -fsS http://127.0.0.1:2410/api/subchunk \
  -H 'Content-Type: application/octet-stream' \
  --data-binary @/tmp/solid.nvblocks \
  -o /tmp/uploaded.nvmesh
```

输入格式或参数无效返回 400；无法确定静态几何返回 422，错误正文使用 JSON。未知示例返回 404。

## 更新与验证

资源由 [viewer-core 的统一生成器](../../neomega-viewer-core/viewer-assets/cmd/build-tables/README.md) 生产。
应用页面通过 `go generate ./viewassets` 构建，然后重新编译 world-viewer。

在工作区根目录运行：

```sh
./check.sh
npm --prefix apps/neomega-viewer/demo/web ci
npm --prefix apps/neomega-viewer/demo/web test
npm --prefix apps/neomega-viewer/demo/web run build
npm --prefix apps/neomega-viewer/cmd/world-viewer/frontend ci
npm --prefix apps/neomega-viewer/cmd/world-viewer/frontend test
npm --prefix apps/neomega-viewer/cmd/world-viewer/frontend run build
```

HTTP 测试覆盖资源、上传、存档切换、诊断、缓存和取消；绘制核心的测试在 viewer-core 中运行。
程序与素材的来源见 [NOTICE.md](NOTICE.md)。
