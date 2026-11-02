# 最小子区块示例

浏览器请求 `GET /api/subchunk?subchunk=default`。Go 在请求内构造一个 16×16×16 的子区块，内容是石头平台上的木屋，带门洞、玻璃窗和阶梯形屋顶。`mesher.MeshSubchunk` 根据内嵌几何表生成网格，默认剔除遮挡面并合并相邻面，再用 `Mesh.Marshal` 编码为 NVMS（viewer 的网格二进制格式）返回。

网页通过共享绘制包的 `decodeMesh` 解码，用 `createMeshObject` 把网格放入 Three.js 场景。方块贴图来自与几何表配套的内嵌图集。画面只在相机改变或窗口缩放时重绘。

## 运行

在本示例目录执行。前端只需在源码变化后重新构建，运行时由 Go 同时提供网页、贴图和网格接口。

```sh
cd neomega-viewer/examples/basic
npm ci
npm run build
go run .
```

打开 <http://127.0.0.1:2413>。拖动鼠标旋转，滚轮缩放，右键拖动平移。需要更换端口时运行 `go run . -listen 127.0.0.1:2414`。

## 阅读代码

从 [main.go](main.go) 的 HTTP handler 开始：`defaultSubchunk` 生成 4096 个 Java 方块状态 ID，随后调用网格生成与编码。ID 是规范化状态的 `Hash()`；本例中的状态没有属性，已经是规范形式。数组按 `(x << 8) | (z << 4) | y` 定位格子，未放方块的位置填空气 ID。

接着读 [main.ts](main.ts)：加载图集、请求网格、解码、创建绘制对象，最后设置相机和鼠标控制。网页的 `await` 用于等待网络结果；Go handler 内的构造、网格生成与编码按顺序执行。

贴图的来源和许可见服务提供的 [THIRD_PARTY_NOTICES.txt](http://127.0.0.1:2413/textures/THIRD_PARTY_NOTICES.txt) 与模块的 [NOTICE.md](../../NOTICE.md)。
