# Java 与 Bedrock 存档浏览器

`world-viewer` 是一个可以单独分发的 Java 与 Bedrock 存档浏览程序。Go 服务只读访问相机附近的区块，再用 viewer 生成网格。Bedrock 方块在内存里转换成 Java 状态，默认经过邻接修复后进入同一条渲染路径，转换和修复结果不写回文件。结构文件会先写成对应的 Java 或 Bedrock 世界目录，再沿同一条区块读取路径显示。浏览器在全屏 Three.js 场景中显示它们，移动时只补入新进入范围的 section。section 是区块中一个 16×16×16 的方块区域。

程序内嵌网页、Three.js、图集、附魔书贴图和几何表。运行时只需要一个与目标操作系统及处理器匹配的可执行文件、一份存档和支持 WebGL 2 的浏览器。没有指定存档时，页面仍是同一个空场景。

## 运行

将编译好的程序复制到目标电脑，在终端启动：

```sh
./world-viewer
```

打开 <http://127.0.0.1:2412>，点击右下角“打开存档…”。对话框列出 Go 服务所在电脑上的目录和文件。双击文件夹进入，使用“上一级”返回；选中世界目录、`level.dat`、Java PackFS 文件，或 `.schem`、`.schematic`、`.litematic`、`.nbt`、`.mcstructure`、`.mcworld` 后点击“打开”。当前目录本身也可以直接打开。包含 `db/` 子目录的世界目录按 Bedrock 打开，即使没有 `level.dat` 也可以使用。选中 `level.dat` 时打开它所在的世界目录。存档数据由 Go 按路径读取，浏览器不上传整个文件。

路径框可以直接输入或粘贴路径。“转到”用于浏览该位置，“打开”用于载入输入的存档。Windows 版支持盘符选择、`D:\Minecraft\我的世界` 这类路径，以及 `\\server\share\世界` 这样的 UNC 路径。路径属于运行 Go 程序的电脑，Windows 使用 `world-viewer.exe`。

载入后，同一个按钮变为“切换存档…”。新存档成功打开后，旧请求会取消，旧网格和缓存会释放；打开失败则保留原存档。也可以从命令行预选：`-world /path/to/java-world`、`-world /path/to/bedrock-world` 或 `-pack /path/to/world.pack`。PackFS 容器以 Java 世界目录为根，只支持 Java 存档。

结构文件先转换再预览：schem、schematic、litematic、nbt 生成 Java 世界，mcstructure、mcworld 生成 Bedrock 世界。输出在源文件旁的同名文件夹中，例如 `城堡.schem` → `城堡/`，不需要额外指定目录。同名文件夹已经是存档时直接复用，以保留其中的编辑。源文件更新后，需要移走或删除旧存档目录再打开；非空且不是存档的目录不会被覆盖。

结构转换默认使用 512 MiB 工作预算；启动时加 `-structure-memory-mib 1024` 可以提高到 1 GiB，最小值为 64 MiB。`-structure-workers` 设置 Java 写出任务数上限，默认 0 按 CPU 自动选择（最多 8 个），仍受预算限制。预算不是进程内存硬上限。这两个参数同时用于启动时和网页中打开的结构文件；渲染并发由独立的 `-workers` 参数控制。

默认加载主世界。Java 优先识别根目录 `region`，其次识别 `dimensions/minecraft/overworld/region`。其他 Java 维度通过 `-dimension` 指定相对于存档根目录的目录，例如 `DIM-1` 或 `dimensions/minecraft/the_nether`。Bedrock 的 `-dimension` 使用编号：0 是主世界，1 是下界，2 是末地，默认 0。

邻接修复根据周围方块补全栅栏连接、楼梯拐角等 Java 属性。需要查看 Bedrock 的直接转换结果时，启动参数加 `-bedrock-fix=false`。这个设置也用于网页中打开的 Bedrock 存档，以及 mcstructure 转出的 Bedrock 存档。修复只发生在预览时，已经写成的 Java 世界不受影响。

初始位置取 `level.dat` 中的 SpawnX、SpawnY、SpawnZ，并向上偏移 12 格。Java 从 Data 内读取，Bedrock 从根读取。没有出生点时先尝试坐标 `(0,80,0)`。该位置没有区块时，定位到所选维度第一个已存区块的最高非空气 section 上方，找到后停止扫描。也可以用 `-position '128,100,-256'` 指定初始坐标。`-listen` 调整监听地址，默认只监听本机。

程序支持 celler 当前的现代 Java section 格式，包括负高度。内嵌几何使用 Java 1.21.4 资源。Java 读取器保留存档原始 blockstate（方块名称及属性），渲染时拆分含水层，再按状态查找几何表。表中包含自动生成的旧状态兼容索引，例如旧 `grass` 共享 `short_grass` 的形状。同一状态采用高版本解释，不依赖存档的 DataVersion，也不保证逐版本还原历史外观。未被表覆盖的 Java 状态显示为紫色方块，面板提示数量。陶罐纹样使用存档中的 block entity NBT，附魔台带有书本动画，其他方块使用几何表中的静态外观。

无法转换的 Bedrock 状态会变成空气，并以 `unknown_block` 按原始状态报告数量；这些方块不计入紫色缺模型数量。不支持的方块实体以 `unsupported_block_entity` 按 ID 汇总。普通展示框与发光展示框使用实体数据中的朝向和物品旋转，框体与普通静态物品共享公共图集。Java 实体从独立的 entities region 读取；Bedrock 使用转换结果中的 Entities。地形缺失或 section 没有方块时，普通实体仍可显示。物品模型按客户端的真实 items 定义选择，复杂选择树、特殊外观和 components 留下实体诊断。

方块实体是用于陶罐纹样等外观的附加数据。Java 空方块实体 List 不论元素类型都视为没有方块实体。Java 方块实体列表格式异常时按没有实体处理，陶罐使用无纹样的默认外观，并保留 `invalid_block_entities` 诊断。单项缺少 Int 坐标或坐标不在该区块时，记录 `invalid_block_entity` 并继续读取其他实体。特殊外观生成失败时保留对应静态本体和水层，并记录 `actor_error`。普通实体列表或位置异常分别产生 `invalid_entities`、`invalid_entity` 诊断。展示框物品外观失败记录 `entity_error`，框体仍保留。区块 NBT、方块调色板等基础地形数据无法读取时仍返回错误。

## 操作与视距

左上角透明区域显示方向轴和当前坐标，坐标下显示帧率与网格内存估算。右下角面板可以收起，提供打开/切换存档、坐标跳转、线框、遮挡剔除、视距和渲染分辨率设置。

- 拖动鼠标转向；`W/A/S/D` 按相机朝向在水平面前后和左右移动，抬头或低头不改变移动高度。
- `Shift` 沿 Y 轴下降，空格沿 Y 轴上升；滚轮沿视线移动，按住 `Shift` 滚动时加大步长。
- 输入 X/Y/Z 后按回车或点击“移动到坐标”。
- “线框”同时作用于当前和新载入的网格。
- “遮挡剔除”默认开启，跳过被不透明表面完全挡住的区域。原地转头可以沿用结果，移动后重新判断。线框模式下暂停剔除；关闭开关可以对比同一位置的效果。
- “渲染分辨率”即时调整画布的绘制像素数，保持界面大小、视距和几何细节不变。默认“自动”跟随屏幕像素密度，最高 1.5 倍。“标准”使用 1 倍，“流畅”使用 0.75 倍，“低分辨率”使用 0.5 倍；“屏幕原生”完整使用屏幕像素密度。

面板中的“渲染异常”可以展开，列出缺失模型或特殊外观生成失败的原始 blockstate、首次发现的世界坐标和原因。渲染问题按完整状态与原因保留一条，含水等属性仍按存档原样显示。普通实体渲染错误按位置保留，显示实体 ID 和详情。来源中的实体数据异常逐条保留；无法定位到具体方块时显示所属区块。整列来源诊断包含请求高度以外的问题，同一列分多次加载不会重复累计。记录来自当前打开存档后实际加载的区域，不代表整张地图的扫描结果；移出视距后保留，重新打开或切换存档时清空。空气等有意没有几何的状态不会被列为异常。

帧率显示最近约一秒的平均值，从后台切回页面后重新采样。网格内存按已加载几何的 CPU 缓冲与同尺寸 GPU 副本估算，包含批绘制缓冲的预留空间，共享缓冲只计一次。数值每秒更新，便于观察视距变化和网格卸载；它不包含纹理、浏览器自身与加载中的临时数据，不代表浏览器进程总内存。悬停在数值上可以查看统计范围。

水平视距默认 8 个区块，最多 30 个区块，即半径 480 格。加载范围是圆形，优先请求最近的区块。垂直方向取相机所在 section 上下各 `min(视距 + 1, 12)` 个 section。

网格按固定的 `(区块 X, section Y, 区块 Z)` 管理。横向移动、上下移动或调大视距都保留重叠部分的原有网格，只读取新增部分。离开范围的网格会释放。同一列连续缺失的 section 合成一次请求，每批最多 15 个，同时最多四次下载。仍有部分内容需要的在途请求会继续完成。

下载与主线程处理分开调度：响应字节收齐后立即释放下载名额，收到的数据等待动画帧解码、准备网格并挂载。每帧为这些操作分配约 4 毫秒，单次解码或一个 section 的处理不可拆分，因此复杂 section 可能超过这个预算。已准备好的 section 立即显示，同列内优先处理靠近相机高度的部分。下载、待解码和正在处理的任务合计最多八个，避免网络过快时积压大量临时数据；移动后不再需要的任务会取消或丢弃。

浏览器按设定视距持续加载完整范围，不设置固定的网格内存预算。内存占用随视距和场景复杂度增长；缩小视距、移动离开原区域或切换存档时，会释放不再需要的网格。

为减少每帧的绘制提交，相邻的 4×4×4 个 section 组成一个空间分组。不透明和镂空几何在组内按兼容的绘制状态批绘制，并在整个世界内共享材质和纹理动画时钟。不同贴图的位置从已有材质数据生成顶点属性，原有面片、颜色、UV 和动画参数保持不变。水和玻璃保留 section 内的材质批次，透明排序仍以这些批次为单位。复杂透明交叠可能出现顺序差异。光照与颜色使用 viewer 的材质规则。

浏览器支持 `WEBGL_multi_draw` 时，同一批中的可见几何范围一次提交；不支持时使用 Three.js 的逐范围绘制回退，画面保持一致，但提交次数较多。

可见性检查先判断整个空间分组：完全在视野外就跳过，完全在视野内就接受；只有与视野边界相交的分组才逐个检查 section。判断使用网格与特效的实际边界，因此能包含越过 section 边缘的形状和附魔书动画。静态几何的位置在挂载时换算到分组坐标，动画单独更新。空 section 只保留加载记录，不加入渲染场景。

视野内的几何还可能被山体或建筑遮住。遮挡剔除使用实际不透明表面的深度，测试空间分组和 section 的包围盒。只有整个包围盒在屏幕内、且 GPU 确认没有任何可见样本时，才跳过该对象。包围盒稍微向外扩张，避免几何自身的共面表面造成误判。镂空、半透明几何和动画特效不提供遮挡深度，但它们可以随所在区域一起被遮挡。

GPU 查询每帧最多发起 8 次，结果异步读取，尚未得到结果的对象正常绘制。需要查询时先绘制不透明表面，再测试包围盒，最后绘制镂空、半透明表面和特效；没有新查询时保持单次绘制。相机位置不变且已加载几何不变时，原地转向沿用已有结果。转向改变像素采样位置，极细缝隙可能出现像素级差异，因此停止转向两帧后清空结果，按当前视角重新验证。相机平移、投影或绘制分辨率变化、网格加入或卸载、切换开关及线框模式时也会清空结果；持续平移期间不发起查询。

共享包的 `SectionScene` 使用viewer-core 的 `viewer/web/render/occlusion.ts` 管理遮挡判断。它使用已加载几何的不透明深度和边界。被挡住的网格仍然驻留，加载范围也保持不变，因此它节省的是绘制开销。开阔地形可能没有明显收益。

每个 section 在共享缓冲中保有独立的几何范围，可以单独隐藏和卸载。缓冲按需增长，大量卸载后会收缩，空分组立即释放。材质在切换或关闭世界时统一释放。这些处理由共享包的 `SectionScene` 管理，实现在viewer-core 的 `viewer/web/render/section-scene.ts`。

## 编译和分发

源码构建需要 Go 1.25.5 或以上。本模块的 `viewassets/bundle/web` 保存应用网页，viewer-core 的 `viewer/resources` 保存可直接嵌入的几何与贴图，因此普通 Go 构建不需要 Node.js 或旁边的 assets 目录。在 `apps/neomega-viewer` 目录执行：

```sh
go build -o cmd/world-viewer/world-viewer ./cmd/world-viewer
```

只分发生成的 `world-viewer` 文件即可。目标电脑需要另外提供要浏览的存档。为其他平台构建时指定 Go 的目标平台，例如：

```sh
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
  -o cmd/world-viewer/world-viewer.exe ./cmd/world-viewer
```

资源和源码许可声明同样包含在程序中，可通过 `/resources/THIRD_PARTY_NOTICES.txt`、`/LICENSE.txt` 和 `/THREE-LICENSE.txt` 读取。

## 独立前端与资源更新

`frontend/` 负责页面、NVWC 列响应和加载调度。为了复用绘制与 section 场景，它通过 `file:../../../../../neomega-viewer-core/viewer/web` 依赖独立包 `@neomega/viewer`，从 `@neomega/viewer/render` 导入绘制接口。Vite 配置使用 `resolve.dedupe: ["three"]` 让应用与共享包使用同一份 Three.js。前端开发需要 Node.js 24 或以上。在 `apps/neomega-viewer` 目录进入前端工程，按锁文件安装依赖并链接共享包：

```sh
cd cmd/world-viewer/frontend
npm ci
npm run dev
```

保持 Go 服务运行，打开 <http://127.0.0.1:2415>。开发服务器把 API 和贴图请求代理到 Go 服务。

修改前端后，在 `frontend/` 执行 `npm run build` 更新模块根目录的 `viewassets/bundle/web`，再重新编译 Go 程序。内嵌文件由 `viewassets/embed.go` 的 `go:embed` 收入程序，运行时通过 `fs.FS` 提供。

更新模型与贴图时，先按 [统一建表说明](../../../../neomega-viewer-core/viewer-assets/cmd/build-tables/README.md) 更新 viewer-core 的配套资源。
页面构建独立于资源生成，在 `apps/neomega-viewer` 模块目录执行：

```sh
go generate ./viewassets
go build -o cmd/world-viewer/world-viewer ./cmd/world-viewer
```

`go generate` 调用本目录的 `bundle.mjs`，安装前端依赖、复制源码许可并构建页面。
生成的 `viewassets/bundle/web` 随源码提交，使普通 Go 构建不需要前端工具链。

## 读取与传输

程序负责打开存档、选择维度、确定初始落点和切换内容。Java 来源通过 `source/javaworld` 串行读取原始记录，各次列请求分别解码。Bedrock 由 converter 的 adapt 层按坐标转换并缓存，`source/bedrockworld` 将转换结果适配为相同的 section 输入。`column` 调用共享 Pipeline 读取六向邻居与四个水平邻居上方的 section 并构建快照，再编码成 NVWC 响应。`viewhttp` 管理并发、响应缓存和静态资源。各块的边界及组装方法见[模块说明](../../README.md#用-section-来源组装浏览服务)。

Java 压缩区块缓存为 32 MiB，最多打开 16 个 region。Bedrock 转换缓存和修复缓存各为 64 MiB。各格式的列响应缓存为 64 MiB。Bedrock 冷请求较慢：渲染一根修复后的柱子需要它和四个水平邻居，共五根修复柱子；每次修复又需要周围 3×3 的转换结果，合起来约需转换 21 根柱子。缓存会复用这些重叠的读取、转换和修复结果。

服务默认允许四个列请求并发构建，可通过 `-workers` 调整；每列内部的 section 仍依次构建。并发调度由 HTTP 层负责，Pipeline 在一次批量调用内缓存 section 准备结果，mesher 接收单个 section 及邻居的渲染层并返回几何。服务按需读取地形与普通实体记录，启动时只探测初始落点。缓存以打开存档时的内容为准。修改存档后，重新选择同一份存档即可重新读取。

`GET /api/world` 返回存档编号、路径、初始坐标、维度和资源版本。`kind` 为 `directory`、`pack`、`bedrock` 或结构文件的扩展名（schem、schematic、litematic、nbt、mcstructure、mcworld）。结构来源的 `path` 保留用户选中的文件路径，区块读取来自旁边的同名原生存档。`GET /api/files?path=...` 浏览服务端目录；`POST /api/world` 接收 JSON `{ "path": "..." }` 打开存档，并返回新的存档编号。`GET /api/chunk?world=1&x=0&z=0&minY=2&maxY=6` 请求指定区块列中的连续 section。参数错误返回 400，存档编号已过期返回 409，无法构建几何返回 422，排队已满返回 503。多个页面共享服务端当前打开的存档，其他页面在切换后刷新即可同步。

区块响应为固定小端的 NVWC v3。20 字节头包含 `NVWC` 标记、uint32 版本、uint32 section 数量、uint32 缺失模型总数和 uint32 诊断 JSON 字节数。每个 section 随后写入 int32 Y、uint32 帧长度、uint32 缺失模型数和完整 NVFR 帧。所有 section 帧之后附带 UTF-8 JSON 诊断数组。无诊断时字节数为零。静态几何、actor 和特效都为空的 section 不写帧，前端将其记为空 section。单次响应上限 24 MiB。NVFR 内部格式见 [快照格式](../../../../neomega-viewer-core/viewer/merger/FORMAT.md)。

诊断需要同时表达某一格的渲染失败和整列的来源汇总，因此使用明确的 `scope` 和 `origin`。例如，下面两条分别表示缺失模型的一格，以及整列来源中无法读取的一项实体：

```json
[
  {
    "scope": "block",
    "origin": "render",
    "position": [-24, -13, 226],
    "blockstate": "example:missing []",
    "reason": "missing_model",
    "count": 1
  },
  {
    "scope": "column",
    "origin": "source",
    "position": [-32, 0, 224],
    "reason": "invalid_block_entity",
    "count": 1,
    "detail": "block_entities[2]: block entity requires Int x/y/z inside its chunk"
  }
]
```

`scope: "entity"` 的 position 是普通实体所在的世界方块坐标，`entityId` 保存实体类型。实体实际渲染位置仍使用浮点坐标。

`scope: "block"` 的 position 是世界方块坐标；`scope: "column"` 的 position 是柱子原点，Y 固定为 0。整列诊断也可以带 blockstate，不能用状态是否为空判断定位范围。`count` 保留该条问题涉及的数量，可选的 `blockEntityId` 用于按实体类型汇总的问题，`detail` 保存具体错误。

`origin: "render"` 的诊断包括 `missing_model` 和 `actor_error`，方块问题按完整状态与原因去重。普通实体的 `entity_error` 按位置、实体 ID 和错误保留，不合并不同位置的展示框。其 count 为首次报告的单格数量 1，NVWC 头和 section 头另行统计全部缺失模型格数。`origin: "source"` 的诊断由来源逐条提供，仅属于请求柱子，覆盖整列高度。前端按柱子坐标、条目内容和相同条目的出现次序消除不同高度窗口之间的重复，保留单次响应内的所有来源条目。来源计数不会增加 NVWC 的缺失模型计数。

工作区根目录的验收命令：

```sh
./check.sh apps/neomega-viewer
npm --prefix neomega-viewer-core/viewer/web ci
npm --prefix neomega-viewer-core/viewer/web test
npm --prefix neomega-viewer-core/viewer/web run typecheck
npm --prefix apps/neomega-viewer/cmd/world-viewer/frontend ci
npm --prefix apps/neomega-viewer/cmd/world-viewer/frontend test
npm --prefix apps/neomega-viewer/cmd/world-viewer/frontend run build
```

测试覆盖文件浏览、存档切换与缓存隔离、退役等待在途构建、Java 目录与 PackFS 读取、结构文件写成 Java／Bedrock 世界后再读取、内存来源出网格、跨 section 剔面和整列诊断归属。Bedrock 测试只打开样例的临时副本，验证转换与修复两种组装能生成网格，并覆盖维度、出生点、目录识别、修复开关、诊断计数和关闭前等待原始读取。前端工程测试移动与视距变化时保留网格、取消过期请求、完整视距加载、列响应解码和跨高度窗口的诊断去重。共享包测试跨 section 批绘制、共享材质生命周期、分层可见性、遮挡查询的异步处理与失效、缓冲收缩、特效动画和绘制二进制解码。
