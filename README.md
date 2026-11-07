# neomega-viewer

`neomega-viewer` 提供 Java／Bedrock 存档浏览器，以及检查方块外形和子区块网格的演示页面。Go 服务按相机位置读取区块，必要时把 Bedrock 状态转换为 Java 状态，再由渲染核心生成网格，浏览器负责显示和交互。

使用存档浏览器只需要编译好的程序、存档和支持 WebGL 2 的浏览器。网页、贴图和几何表已经内嵌；从源码编译需要 Go 1.25.5 或以上，并保留本工作区的兄弟模块目录，依赖路径由 `go.mod` 指定。

## 浏览存档

以下命令都在 `apps/neomega-viewer` 目录运行：

```sh
go build -o cmd/world-viewer/world-viewer ./cmd/world-viewer
./cmd/world-viewer/world-viewer
```

打开 <http://127.0.0.1:2412>，点击右下角“打开存档…”，选择服务所在电脑上的 Java／Bedrock 世界目录、`level.dat`、Java PackFS 文件，或 `.schem`、`.schematic`、`.litematic`、`.nbt`、`.mcstructure`、`.mcworld`。PackFS 是把世界目录保存成单文件的容器。也可以在启动时直接指定：

```sh
./cmd/world-viewer/world-viewer -world /path/to/java-world
./cmd/world-viewer/world-viewer -world /path/to/bedrock-world
./cmd/world-viewer/world-viewer -pack /path/to/world.pack
./cmd/world-viewer/world-viewer \
  -world /path/to/build.schem
```

结构文件先转换再预览：schem、schematic、litematic、nbt 生成 Java 世界，mcstructure、mcworld 生成 Bedrock 世界。输出在源文件旁的同名文件夹中，例如 `城堡.schem` → `城堡/`，不需要额外指定目录。同名文件夹已经是存档时直接复用，以保留其中的编辑。源文件更新后，需要移走或删除旧存档目录再打开；非空且不是存档的目录不会被覆盖。

结构转换默认使用 512 MiB 工作预算；启动时加 `-structure-memory-mib 1024` 可以提高到 1 GiB，最小值为 64 MiB。`-structure-workers` 设置 Java 与 mcstructure 写出任务数上限，默认 0 按 CPU 自动选择（最多 8 个），仍受预算限制。预算不是进程内存硬上限。这两个参数同时用于启动时和网页中打开的结构文件；渲染并发由独立的 `-workers` 参数控制。

Bedrock 方块默认在内存中转换并修复栅栏连接、楼梯拐角等邻接属性，结果不写回存档。加 `-bedrock-fix=false` 可以查看直接转换的结果。这个开关也用于转换后的 mcstructure 存档预览，修复结果只留在内存。Bedrock 维度用 `-dimension 0`、`1`、`2` 选择主世界、下界、末地；Java 维度用存档内相对目录，例如 `-dimension DIM-1`。结构文件忽略 `-dimension`。

拖动鼠标转向，`W/A/S/D` 水平移动，空格上升，`Shift` 下降。右下角面板提供坐标跳转、线框、视距和渲染分辨率。视距默认 8 个区块，增大视距会增加内存与绘制开销。

几何资源与 `neomega-blocks/presets/java/v1.JavaTargetVersion` 定义的 Java 状态目标版本一致（当前为 26.3）。未覆盖的 Java 状态显示为紫色方块；无法转换的 Bedrock 状态会变成空气，并在“渲染异常”中报告。异常面板只统计实际加载的区域。更完整的参数、读取范围与诊断说明见 [存档浏览器说明](cmd/world-viewer/README.md)。

## 下载编译版本

从 [Releases](https://github.com/CMA2401PT/neomega-viewer/releases) 下载。每个形如 `v0.0.10` 的版本标签对应一个 Release，包含三个平台的下载包：

- macOS arm64：`world-viewer-macos-arm64.tar.gz`。
- Windows amd64：`world-viewer-windows-amd64.zip`。
- Linux amd64：`world-viewer-linux-amd64.tar.gz`。

压缩包内包含可独立运行的程序，解压后按前面的“浏览存档”说明启动。版本 Release 中的下载包长期保留。

另有一个浮动的 [latest Release](https://github.com/CMA2401PT/neomega-viewer/releases/tag/latest)，由手动工作流更新，用来获取从分支构建的最新版本；它的版本号显示为 `dev-<短提交>`。

### 发布新版本

程序里不再写死版本号，版本只来自 Git 标签。把要发布的提交推送到 `main` 后，创建并推送版本标签：

```sh
git tag -a v0.0.10 -m "v0.0.10"
git push origin v0.0.10
```

工作流自动构建三个平台。全部成功后创建与标签同名的 Release 并上传压缩包；构建时用 `-ldflags "-X main.version=v0.0.10"` 把标签写进程序，因此终端开场和网页右下角面板都会显示这个版本号。标签必须形如 `v1.2.3`，重复运行同一标签会替换已有 Release 的下载包。

本地直接 `go build` 不带该参数，版本显示为 `dev`；只查看版本号可以运行 `world-viewer -version`。

需要从分支构建而不发版时，在 [Actions 页面](https://github.com/CMA2401PT/neomega-viewer/actions/workflows/build.yml) 打开“编译并发布存档浏览器”，点击 **Run workflow** 并选择分支。该路径不创建版本 Release，成功后更新 `latest`。本次运行的 **Artifacts** 也保留一份下载包，保留期为 30 天。

工作流使用标签或所选分支的应用源码，以及同一 GitHub 账户下各依赖仓库的默认分支。依赖按本地工作区的目录层级取齐，以满足 `go.mod` 中的 `replace` 路径。Go 版本取自应用的 `go.mod`，三个平台均关闭 CGO 并使用 Go 交叉编译。网页和几何资源直接使用已提交的内嵌产物。

## 检查单个方块与网格

需要比较方块状态、面剔除与网格合并时，运行演示页面。它需要 Node.js 24 或以上；网页构建后与 Go 服务一起使用：

```sh
npm --prefix demo/web ci
npm --prefix demo/web run build
go run ./demo/cmd/neomega-viewer
```

打开 <http://127.0.0.1:2410> 搜索方块和选择状态；打开 <http://127.0.0.1:2410/subchunk.html> 比较场景、上传逻辑方块层或下载网格。演示服务默认读取兄弟模块 `neomega-viewer-core/viewer/resources/textures` 的配套图集。

资源或页面位于其他位置时，可以明确传入路径：

```sh
go run ./demo/cmd/neomega-viewer \
  -assets /path/to/runtime/textures \
  -tables /path/to/geometry.nvtables \
  -web /path/to/web/dist \
  -listen 127.0.0.1:2410
```

## 用 section 来源组装浏览服务

模块导入路径是 `neomega-viewer`。如果已有自己的存档或结构格式，可按下列边界接入，而不必改动网页的区块加载方式。一个 section 是 16×16×16 的方块区域。

| 入口 | 调用方用它完成什么 |
|---|---|
| `source.MemorySections` | 把内存中的 Java 方块状态和实体交给渲染流程 |
| `source/javaworld` | 将已打开的 Java 世界适配为 section 来源 |
| `source/bedrockworld` | 将 converter 的按坐标转换结果适配为相同来源 |
| `column.BuildColumn` | 构建指定高度范围并编码为网页使用的 NVWC 列响应 |
| `viewhttp.NewView`、`viewhttp.Handler` | 提供一代内容的响应缓存、并发调度与 HTTP 服务 |
| `viewassets.Web` | 取得已经构建好的浏览页面 |

来源数据、调色板和实体 NBT 应保持只读。内容改变时创建新的 View，切换后先等待旧构建及底层读取退出，再关闭存档句柄。调用取消不代表底层读取已经结束。具体组装、来源所有权和生命周期见 [开发说明](AGENTS.md#用-section-来源组装浏览服务)。

只需要网格生成或 Web 绘制接口时，从 [neomega-viewer-core](../../neomega-viewer-core/README.md) 开始。维护入口、前端构建、资源更新与验收命令见 [AGENTS.md](AGENTS.md)。程序与素材的来源见 [NOTICE.md](NOTICE.md)。
