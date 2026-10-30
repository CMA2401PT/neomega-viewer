# 特殊方块：现状与待办

viewer 的输入是一个 16×16×16 的 LayerStorage：每格一个 Java block hash，没有 NBT。下文把这个输入格式叫
**本表达空间**。在本表达空间里，现有方块都能渲染对：给出正确的 Java 状态，渲染器就能画对，含水方块也已经能叠加
水层（见 `多层渲染方案.md`）。

剩下的问题不在渲染器本身，分成三类：

1. **映射丢信息**：从其他表达空间（NEMC/Bedrock）映射到本表达空间时，转换器丢了信息。这是转换器的问题，不是渲染器的问题。
   按丢掉的信息还能不能找回来，再细分两类：
   - **1a. 能从本表达空间补回来**：丢掉的属性可以由周围格子推出来。这是 neomega-converter 里 `fixer` 的职责。
   - **1b. 信息已彻底丢失**：周围格子推不出来，只能走别的路径，例如改转换器、或读取原存档的 NBT。还没有解决。
2. **NBT 方块**：在 Java 里，关键信息本来就只存在 block entity NBT 里，本表达空间装不下。架构已初步具备。
3. **代码驱动渲染的方块**：Java 用 block entity renderer（代码）画出来的部分，静态模型管线画不了。架构已初步具备。

分清是哪一类，就知道该去哪里改：1a 改 neomega-converter 的 `fixer`，1b 改 neomega-converter 的转换器，2 加一个 block actor 处理器，3 加一个前端特效。
2、3 的通道设计见 `渲染分发方案.md`。

Java 事实以仓库目标版本 Java 1.21.4 为准，拿不准的地方已标注。

## 1. 映射丢信息

这一类可以整批扫出来：把每个 Java 状态做一遍 Java→NEMC→Java 往返，回来和原状态不一致的，就是转换丢了信息。
往返工具在 `neomega-blocks/presets/block-converts/scripts/java-roundtrip`，结果在它的 `REPORT.md`。往返只能
告诉我们丢了什么，至于能不能补回来，要逐项判断，判断结果就是下面的 1a / 1b。

### 1a. 能从本表达空间补回来（fixer 负责）

判据：丢掉的属性完全由当前格周围的方块决定，看邻居就能重算。每项的规则和依据见 `neomega-converter/fixer/FIXER.md`。

- [x] 栅栏、玻璃板、铁栏杆的连接方向：由四周是否有可连接的方块决定。
- [x] fire 的附着方向：由四周和上方哪些面可燃决定。
- [x] 楼梯拐角 `shape`：由前后两格的楼梯朝向决定。`special_nemc` 开启修复后，L 形台阶的拐角与 `special_java` 一致。
- [x] 红石线连接：只做了保守版，原因见 `neomega-converter/fixer/FIXER.md`。
- [x] 门：上下两半各自丢掉一部分属性，正好互补。往返实测：下半格丢 `hinge`，上半格丢 `facing` 和 `open`，两半都丢
  `powered`。所以上半格的 `facing`、`open` 从下半格取，下半格的 `hinge` 从上半格取。`special_nemc` 开启修复后，
  四扇门八格与 `special_java` 一致。
- [x] 含雪方块的 `snowy`：草方块、菌丝、灰化土的 `snowy` 往返后丢失（`REPORT.md` 共 3 个状态）。原版里它只表示
  正上方是否有雪，看上方一格就能补。

### 1b. 信息已彻底丢失（进行中）

判据：周围格子推不出来，信息只在源数据里。要解决，转换时就得读源数据：输入是 Bedrock 的方块 hash 加 NBT，输出是
Java 的方块 hash 加 NBT，由 neomega-converter 负责。Bedrock 的含水是存档里的第二方块层，所以输入是两层 hash，输出
一层带 `waterlogged` 的 Java hash，交给 viewer 的 splitlayer 再拆。

从 Bedrock 存档实测确认的数据位置：

- **床的颜色**：方块状态里只有 `minecraft:bed`，颜色在 block entity 的 `color` 字段（byte，染料编号，例如 8 浅灰、
  11 蓝）。`special_nemc` 里的蓝、黄、绿床往返后都变成了 `red_bed`。
- **花盆里的植物**：方块是 `minecraft:flower_pot`，植物在 block entity 的 `PlantBlock` 字段，存的是完整的方块 NBT，
  例如 `{name: "minecraft:dandelion", states: {}}`。Java 要合成 `potted_dandelion` 这样的方块名。
- **门、栅栏门、活板门的 `powered`**：表示是否被红石供能，要做红石信号传播才知道，邻居几何给不了（72 种方块、
  1943 个状态）。它不影响外观，渲染上可以接受丢失。

## 2. NBT 方块（架构已初步具备）

判据：在 **Java 的方块表达**里，关键信息也只存在 block entity NBT 里，不在 block state 里。LayerStorage 没有 NBT，
这些信息必然丢失，转换器也补不回来。

判据看的是 Java 侧。床的颜色在 Bedrock 里存 NBT，在 Java 里编进了方块名（`red_bed`、`blue_bed`），所以它属于 1b，不属于这里。

- **旗帜的图案**：颜色在方块名里，朝向在 block state 里，图案只在 NBT 的 `patterns` 列表里。现在只能画出纯色旗帜。
- **展示框**：在 Java 里，展示框是实体（entity），不是方块，本身就不在 LayerStorage 里，里面的物品更不在。
  Bedrock 的展示框是带 block entity 的方块，映射到 Java 时要从方块层挪到实体层。这一点凭记忆，未联网核实。

通道已经有了：输入除了 hash 层，还带一份按格子坐标索引的 SNBT，Dispatcher 把受支持的 NBT 方块交给 block actor，由它按
NBT 算出几何。每种方块是 `blockactor/` 下的一个子包，新增一种只加一个包。

- [x] 陶罐：四面纹样按 `sherds` 换贴图，NBT 结构见 `blockactor/decoratedpot/NBT.md`。
- [ ] 旗帜：要按 `patterns` 合成纹理，并让前端按材质选贴图。
- [ ] 箱子、潜影盒：不做开合动画，有静态外形，目前由 mesher 画。
- [ ] 展示框：Java 里是实体，不在方块层，要另走实体通道。

## 3. 代码驱动渲染的方块（架构已初步具备）

判据：Java 不用静态模型画它，而是由 block entity renderer 按代码绘制，往往还带动画。

只支持完全由前端驱动的效果：Go 只发「某格放某特效」的消息，不定时更新，也不读周围的世界。本体照常由 mesher 画，
特效叠在上面。新增一种效果，就在前端特效注册表里加一个工厂，并在组装处把方块名映射到特效名。

- [x] 附魔台的书：漂浮、转动、翻页。
- [ ] 海晶灯的光点、下界传送门的效果：符合判据，还没做。
- 信标光柱、conduit：需要读子区块外的方块（染色玻璃、框架），不符合判据，目前不支持。

## 对照组：本来就没问题的方块

用来确认「不是所有特殊方块都出问题」：

- 海晶灯：无状态，正常。
- 附魔台本体（不含书）：正常。
- 两格植物（向日葵等）、花瓣、虞美人：往返一致，正常。

对照组如果也出问题，说明问题在转换或渲染的公共路径上，不在「特殊方块」这个范畴。

## 配套场景

`special_java` 和 `special_nemc` 的方块布局相同，可以逐格对照：

- `special_java`：每个方块直接用正确的 Java 状态摆放。
- `special_nemc`：同一批方块经过 Java→NEMC→Java 往返后再摆放。在页面上打开修复开关，就能看 fixer 补回了多少。

同一位置两个场景都错，是第 2、3 类（Java 侧本来就装不下或画不了）；只有 `special_nemc` 错，是第 1 类。开了修复
还错，是 1b，或者是 fixer 还没覆盖的 1a。实现见 `demo/scenes/scenes.go`。

含水方块的叠加效果在 `waterlogged` 和 `water_cube` 场景里看，方块实体与特效在 `block_entities` 场景里看。
