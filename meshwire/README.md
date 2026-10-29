# NVMS v1：传输已经生成的几何

子区块顶点适合以数值缓冲传输，避免先转换成数值文本，再在浏览器构造普通数组。`meshwire.Encode` 将 `geometry.Buffers` 编码成 NVMS；Go 消费者使用 `meshwire.Decode`，浏览器使用 `@neomega/viewer/render` 的 `decodeMesh`。

协议只描述绘制数据，不保存方块 ID、模型树或 Palette。需要重新解析方块时，应保存输入层，而不是试图从顶点反推方块。HTTP 响应类型为 `application/vnd.neomega.mesh`，同样的正文可以保存为 `.nvmesh`。

## 文件头与材质

所有整数为小端无符号 32 位，浮点数为小端 IEEE 754 float32。文件头固定 44 字节：

| 字节偏移 | 类型 | 含义 |
|---:|---|---|
| 0 | 4 字节 ASCII | `NVMS` |
| 4 | uint32 | 版本，值为 1 |
| 8 | uint32 | 材质 JSON 的 UTF-8 字节数，不包含对齐填充 |
| 12 | uint32 | 材质批次数 |
| 16 | uint32 | 总四边形数 |
| 20 | 3 × float32 | 最小坐标 X、Y、Z |
| 32 | 3 × float32 | 最大坐标 X、Y、Z |

文件头后是 `[]material.Surface` 的 JSON。第 i 个材质对应第 i 个批次，包含纹理名、图集区域和透明绘制规则。材质涉及字符串和少量字段，保留 JSON 便于扩展；大量顶点数据仍使用二进制。JSON 后补零至下一个 4 字节边界。

## 批次与数值缓冲

接下来依次写入每个批次。批次头包含 `vertexCount` 和 `indexCount` 两个 uint32，后面的属性缓冲按固定顺序紧密排列：

| 缓冲 | 元素类型 | 元素数 |
|---|---|---:|
| positions | float32 | vertexCount × 3 |
| normals | float32 | vertexCount × 3 |
| shadeNormals | float32 | vertexCount × 3 |
| uvs | float32 | vertexCount × 2 |
| colors | float32 | vertexCount × 3 |
| indices | uint32 | indexCount |

每个四边形依次占连续四个顶点，其索引为 `[base, base+1, base+2, base, base+2, base+3]`。顶点数必须是 4 的倍数，索引数等于 `vertexCount / 4 × 6`。全部批次结束后即为文件结尾，总四边形数必须与文件头一致。

坐标以一个方块为一单位。法线表示真实几何方向，`shadeNormals` 表示用于静态照明的方向。UV 是纹理自身的局部坐标，顶部原点；允许超出 `[0,1]`，用于合并面上的纹理重复。颜色为线性 RGB，材质区域使用整个图集的归一化坐标。

材质区域取决于生成时的图集布局，消费者应加载同一份 PNG。即使 Java 版本相同，不同覆盖包或生成输入也可能改变布局。离线分发时，应将网格、配套图集和来源声明一起保存。

## 如何复用

Go 编码和解码都不依赖 HTTP：

```go
encoded, err := meshwire.Encode(result.Mesh.Buffers())
// encoded 可以写文件，或直接作为 HTTP 正文。
decoded, err := meshwire.Decode(encoded)
```

浏览器在小端平台上直接建立属性视图，材质表单独解析：

```ts
const data = decodeMesh(await response.arrayBuffer());
const mesh = createMeshObject(data, atlas);
```

这里共享的是 CPU 上收到的 `ArrayBuffer`。Three.js 在创建 GPU 缓冲时仍需要上传数据。将一个半透明批次拆成可排序的面时，适配层使用 `subarray` 保留属性视图，避免为每个面复制整份网格。

方块层的 `.nvblocks` 与 NVMS 是两种不同用途的数据：前者保存 4096 个实际 ID，可重新解析和改变网格策略；后者保存一次解析得到的静态几何。方块层格式与可运行示例见 [主 README](../README.md#直接使用-go-库)。
