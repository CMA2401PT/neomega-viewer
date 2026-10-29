# NVGT v2 几何表格式

NVGT 保存渲染需要的两张表：数值 block ID 到 ModelID 列表的索引，以及 ModelID 到 Shape 的几何。一个 Shape 是可共享的局部四边形集合，附带消面组和六方向边界覆盖。运行时查询这些表，再按邻居覆盖裁切、合并。文件扩展名为 `.nvtables`。

多个面常用相同材质、顶点、法线和 UV。文件将材质、Vec3、Vec2 各保存一份字典，四边形通过从零开始的 uint32 下标引用它们。向量按 IEEE 754 位模式去重，保留负零；这使文件紧凑，同时保留全部原始浮点精度。解码后向量和材质恢复为 Shape 内的值，不增加运行时查表层级。

## 数值和顺序

所有多字节数值均为**小端**。`u8/u16/u32/u64` 是对应位宽的无符号整数；`f32/f64` 是 IEEE 754 binary32/binary64。记录之间没有填充、对齐字节或 Go 内存布局。字符串为 `u32 字节长度 + UTF-8 字节`，没有结尾零字节，长度不包含前缀。

文件按以下顺序连续排列：

```text
32 字节头
Surface 字典
Vec3 字典
Vec2 字典
block ID 索引表
Shape 表
FullHeight 映射
```

除头部外，以下偏移均相对于当前记录开头。变长记录的下一条紧接上一条结尾。所有数量和引用下标都是 u32；只有 block ID 按头部选择 u32 或 u64。读取器通过数量推进游标，不需要节区偏移目录。

## 文件头（32 字节）

| 偏移 | 宽度 | 类型 | 含义 |
|---:|---:|---|---|
| 0 | 4 | bytes | ASCII `NVGT`（`4e 56 47 54`） |
| 4 | 2 | u16 | 格式版本，当前为 2 |
| 6 | 1 | u8 | block ID 字节宽度 W，4 或 8 |
| 7 | 1 | u8 | 保留，编码器写 0 |
| 8 | 4 | u32 | 索引条目数 I |
| 12 | 4 | u32 | Shape 数 S |
| 16 | 4 | u32 | FullHeight 映射数 F |
| 20 | 4 | u32 | Surface 数 M |
| 24 | 4 | u32 | Vec3 数 V3 |
| 28 | 4 | u32 | Vec2 数 V2 |

解码器校验 magic、version 和 ID 宽度。Go 的 `DecodeTables[B]` 要求头中的 W 与 B 的底层宽度相同，不做截断或自动拓宽。版本是格式版本，与 Java 资源版本无关。

## Surface 字典（M 条）

材质定义如何采样图集以及透明度、双面和光照行为。四边形存其字典下标，多个 Shape 可以共用同一条材质。动画纹理的 Region 覆盖竖排的全部帧；绘制端用帧数和每帧时长选择其中一帧。

令 T 为 Texture 的 UTF-8 字节数，K 为 Mode 的 UTF-8 字节数：

| 偏移 | 宽度 | 类型 | 含义 |
|---:|---:|---|---|
| 0 | 4 | u32 | T |
| 4 | T | UTF-8 | Texture，即纹理资源名字符串，例如 `minecraft:block/stone` |
| 4+T | 4 | u32 | K |
| 8+T | K | UTF-8 | Mode，当前使用 `opaque`、`cutout`、`blend` |
| 8+T+K | 16 | 4 × f32 | Region：u、v、width、height；归一化图集区域，原点在左上 |
| 24+T+K | 1 | u8 | DoubleSided：0=false，1=true |
| 25+T+K | 1 | u8 | Unlit：0=false，1=true |
| 26+T+K | 4 | f32 | Opacity |
| 30+T+K | 4 | u32 | Frames：帧数，静态纹理为 1 |
| 34+T+K | 4 | u32 | FrameTime：每帧 tick 数，每 tick 为 1/20 秒，静态纹理为 0 |

一条 Surface 共 `38+T+K` 字节。字符串使用长度前缀直接编码，不是 JSON。Frames 和 FrameTime 直接来自图集元数据，并参与 Surface 去重。v1 缺少这两个字段，需要用配套图集重新生成 v2 表。

## 向量字典

Vec3 字典有 V3 条，每条固定 24 字节：偏移 0、8、16 依次是 x、y、z 的 f64。位置、几何法线和光照法线共用这张字典。

Vec2 字典有 V2 条，每条固定 16 字节：偏移 0、8 依次是 u、v 的 f64。它保存四边形的纹理局部 UV，允许超出 0…1 以表达纹理重复。

坐标以一个方块为一单位，Y 向上，北向为 −Z。保留 float64 是因为这些值还参与几何裁切、UV 相容性和合并判断，进入 GPU 缓冲时才转成 float32。

## block ID 索引表（I 条）

| 偏移 | 宽度 | 类型 | 含义 |
|---:|---:|---|---|
| 0 | W | u32 或 u64 | block ID |
| W | 4 | u32 | 部件数 N |
| W+4 | 4×N | N × u32 | 按拼装顺序排列的 ModelID |

条目大小为 `W+4+4×N`。ModelID 是 Shape 表中的零基下标。N=0 表示已知的空几何；索引中缺少某个 block ID 表示无法查询其静态几何。部件顺序须保留。编码器按 block ID 升序写条目，ID 不要求连续。

## Shape 表（S 条）

Shape 的 ModelID 就是其记录序号，不另外存编号。每条记录开头为：

| 偏移 | 宽度 | 类型 | 含义 |
|---:|---:|---|---|
| 0 | 4 | u32 | CullGroup：同种透明介质共用的非零消面组，0 无此分组 |
| 4 | 4 | u32 | 四边形数 Q |
| 8 | 57×Q | Quad[Q] | 以下固定布局的四边形 |
| 8+57×Q | 变长 | 六组 Boundary | 按 west、east、down、up、north、south 顺序 |

### Quad（57 字节）

| 偏移 | 宽度 | 类型 | 含义 |
|---:|---:|---|---|
| 0 | 16 | 4 × u32 | Positions，依顶点顺序引用 Vec3 字典 |
| 16 | 4 | u32 | Normal，引用 Vec3 字典 |
| 20 | 4 | u32 | ShadeNormal，引用 Vec3 字典；所引用的零向量表示使用 Normal |
| 24 | 16 | 4 × u32 | UV，依顶点顺序引用 Vec2 字典 |
| 40 | 12 | 3 × f32 | Color：RGB |
| 52 | 4 | u32 | Surface 字典下标 |
| 56 | 1 | u8 | CullFace：0=不可剔除，1=west，2=east，3=down，4=up，5=north，6=south |

顶点顺序保留，输出三角形使用 `(0,1,2)`、`(0,2,3)`。CullFace 只是允许检查的邻居方向，是否实际遮挡仍取决于边界覆盖。

### 每个方向的 Boundary

先写 `u32 矩形数 R`，再连续写 R 条 36 字节记录：

| 偏移 | 宽度 | 类型 | 含义 |
|---:|---:|---|---|
| 0 | 4 | u32 | Quad：所属 Shape 内的四边形下标，用于取得覆盖材质 |
| 4 | 16 | 2 × f64 | Min：二维矩形最小坐标 |
| 20 | 16 | 2 × f64 | Max：二维矩形最大坐标 |

这里的 Min/Max 直接保存数值，不引用 Vec2 字典。法向轴为 axis（X=0、Y=1、Z=2）时，矩形坐标轴依次为 `(axis+1)%3`、`(axis+2)%3`：X 面为 YZ，Y 面为 ZX，Z 面为 XY。

空方向仍写一个值为 0 的 u32。因此一条 Shape 的总长度为 `8+57×Q+24+36×六方向矩形总数`。

## FullHeight 映射（F 条）

每条固定 8 字节：偏移 0 为原 ModelID（u32），偏移 4 为满高度 ModelID（u32）。编码器按原 ModelID 升序写入。

查询到带此映射的形状后，如果上方也有满高度映射且满高度形状的 CullGroup 相同，就切换当前格为满高度形状。映射不携带流体名称或 Java 属性。

## Go 接口与 Java 演示数据

`tables.Encode(w io.Writer)` 写出上述格式；`mesher.DecodeTables[B](r io.Reader)` 读取它。两者都返回 I/O 错误，文件或 embed 由调用者打开。`mesher.NewMesher(tables)` 持有只读表并提供 `MeshBlock(id)` 与 `MeshSubchunk(blocks, options)`，其中 blocks 为按 `(x<<8)|(z<<4)|y` 排列的 4096 个 ID 的切片。

仓库生成器以 uint32 的 Java hash 作为 block ID。Java 身份由 `blockstate.Java()` 提供。演示所需的资源 Version、Renderers、Skipped 单独写入 `<表路径>.java.json`，其中 Skipped 记录 Binding 和 Reason。这个旁文件不属于 NVGT，独立使用 mesher 不需要它；演示加载器 `demo/blockmesh` 为 JSON 响应加载它。两张几何表的全部内容都在 NVGT 二进制中。
