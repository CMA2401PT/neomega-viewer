# 陶罐的 NBT 结构

陶罐（`minecraft:decorated_pot`）的四个侧面各自可以有一种纹样。纹样不在方块状态里，而在 block entity 的 NBT 里，所以
陶罐由 block actor 渲染：先读出四面的纹样，再给几何换上对应的贴图。本文说明处理器读的是哪些字段，以及每一面朝向哪里。

## 方块状态

| 属性 | 取值 | 对渲染的影响 |
|---|---|---|
| `facing` | `north` / `east` / `south` / `west` | 决定四个侧面的朝向，见下文 |
| `cracked` | `true` / `false` | 不影响外观 |
| `waterlogged` | `true` / `false` | 由 splitlayer 拆出水层，陶罐本体不受影响 |

## block entity NBT

下面的陶罐样本来自 Java 26.3 存档（`DataVersion 5023`），坐标字段已省略：

```text
{
  id: "minecraft:decorated_pot",
  sherds: {
    back:  {id: "minecraft:angler_pottery_sherd"},
    left:  {id: "minecraft:archer_pottery_sherd"},
    right: {id: "minecraft:blade_pottery_sherd"},
    front: {id: "minecraft:arms_up_pottery_sherd"}
  },
  item: {...},
  keepPacked: 0b,
  components: {}
}
```

渲染只用 `sherds`：

- 它是一个 compound（按键组织的 NBT 复合结构），键为 `back`、`left`、`right`、`front`，每个值是一个物品，处理器取物品的 `id`。
- `id` 为 `minecraft:<名字>_pottery_sherd` 时，这一面贴 `entity/decorated_pot/<名字>_pottery_pattern`。
- 其他情况贴素面 `entity/decorated_pot/decorated_pot_side`，包括 `minecraft:brick`、缺少某个键、整个 `sherds` 缺失，以及实体 NBT 缺失。

`item` 是陶罐里装的物品，`keepPacked` 和 `components` 是通用的 block entity 字段，都和外观无关。

### 列表形式

处理器也接受四个物品 id 的 `sherds` 列表，顺序是 back、left、right、front：

```text
{sherds: ["minecraft:angler_pottery_sherd", "minecraft:archer_pottery_sherd",
          "minecraft:blade_pottery_sherd", "minecraft:arms_up_pottery_sherd"]}
```

处理器同时接受 compound 和列表形式，结果相同。上面的 26.3 样本只能确认该版本使用 compound。Java 1.21.4 是否已使用 compound 尚未核实。

## 四个面朝向哪里

**front 朝向 `facing` 的反方向。** 例如 `facing=south` 的陶罐，front 朝北。left、right 以面对 front 的观察者为准：

| `facing` | front | back | left | right |
|---|---|---|---|---|
| `north` | 南 | 北 | 西 | 东 |
| `east` | 西 | 东 | 北 | 南 |
| `south` | 北 | 南 | 东 | 西 |
| `west` | 东 | 西 | 南 | 北 |

依据是两方面：

- 存档实测：上面那个陶罐是 `facing=south`，玩家站在它北面放置；举臂纹样（front）朝北，对着放置它的玩家。
- Java 的 `DecoratedPotRenderer` 把 front 画在模型的 +Z 面，再绕 Y 轴旋转 `180° - facing.toYRot()`，结果与上表一致。

浏览器里 `block_entities` 场景的四个纹样陶罐，用的就是这份存档数据，分别朝向四个方向。

## 贴图映射

纹样贴图是 16×16，陶罐身体的侧面宽 14、高 16。处理器取纹样贴图中间的 14×16 区域（U 为 1/16 到 15/16，V 为 0 到 1），
与原版渲染器的 `texOffs(1, 0)` 一致，保证一个像素对应一个模型单位。只替换身体的四个侧面，瓶颈、瓶口和顶底面保留几何表
里的原样。
