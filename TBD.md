# 特殊方块：现状与待办

下文把一个 16×16×16、每格只存 Java block hash 的方块层叫作**本表达空间**。它表达方块状态，NBT 需要单独传递。
静态 mesher 接收数值 ID 切片；完整 Java hash 先由 splitlayer 拆成单层状态，再交给它。几何表支持的静态状态可按 hash
查表渲染，含水方块可叠加水层（见 [多层渲染方案.md](../../neomega-viewer-core/viewer/多层渲染方案.md)）。`moving_piston` 缺少决定移动内容的数据，
无法仅凭状态确定静态外形。

带 NBT 的渲染使用 `dispatcher.Pipeline`，输入包含完整 Java 状态与按格子坐标索引的已解码 NBT。应用中的 SNBT 是 NBT 的文本表示，解析后再进入核心。

特殊外观所需的信息与处理方式分成三类：

1. **映射丢信息**：从其他表达空间（NEMC/Bedrock）映射到本表达空间时，转换器丢了信息。这是转换器的问题，不是渲染器的问题。
   按丢掉的信息还能不能找回来，再细分两类：
   - **1a. 能从本表达空间补回来**：丢掉的属性可以由周围格子推出来。这是 neomega-converter 里 `fixer` 的职责。
   - **1b. 只能从源数据补回**：周围格子推不出来，需要转换器读取源 NBT 或第二方块层。床颜色、花盆植物和含水转换已实现。
2. **NBT 方块**：在 Java 里，关键信息本来就只存在 block entity NBT 里，本表达空间装不下。架构已初步具备。
3. **代码驱动渲染的方块**：Java 用 block entity renderer（代码）画出来的部分，静态模型管线画不了。架构已初步具备。

分清是哪一类，就知道该去哪里改：1a 改 neomega-converter 的 `fixer`，1b 改 neomega-converter 的转换器，2 加一个 block actor 处理器，3 加一个前端特效。
2、3 的通道设计见 `渲染分发方案.md`。

Java 事实以仓库目标版本 Java 1.21.4 为准，拿不准的地方已标注。

## 1. 映射丢信息

这一类可以整批扫出来：把每个 Java 状态做一遍 Java→NEMC→Java 往返，回来和原状态不一致的，就是转换丢了信息。
往返工具在 `neomega-blocks/third_party/gen_resource/java-roundtrip`，结果在它的 `tmp/REPORT.md`。往返只能
告诉我们丢了什么，至于能不能补回来，要逐项判断，判断结果就是下面的 1a / 1b。

### 1a. 能从本表达空间补回来（fixer 负责）

判据：丢掉的属性完全由当前格周围的方块决定，看邻居就能重算。每项的规则和依据见 `neomega-viewer-core/fixer/java-postprocess/FIXER.md`。

- [x] 栅栏、玻璃板、铁栏杆的连接方向：由四周是否有可连接的方块决定。
- [x] fire 的附着方向：按四周及上方邻居朝向火的那一面是否实心，近似重算。
- [x] 楼梯拐角 `shape`：由前后两格的楼梯朝向决定。`special_nemc` 开启修复后，L 形台阶的拐角与 `special_java` 一致。
- [x] 红石线连接：只做了保守版，原因见 `neomega-viewer-core/fixer/java-postprocess/FIXER.md`。
- [x] 门：上下两半各自丢掉一部分属性，正好互补。往返实测：下半格丢 `hinge`，上半格丢 `facing` 和 `open`，两半都丢
  `powered`。所以上半格的 `facing`、`open` 从下半格取，下半格的 `hinge` 从上半格取。`special_nemc` 开启修复后，
  四扇门八格与 `special_java` 一致。
- [x] 含雪方块的 `snowy`：草方块、菌丝、灰化土的 `snowy` 往返后丢失（`REPORT.md` 共 3 个状态）。原版里它只表示
  正上方是否有雪，看上方一格就能补。

### 1b. 只能从源数据补回（Bedrock 转换器负责）

判据：周围格子推不出来，信息只在源数据里。`neomega-converter-core/bedrock` 读取两层 Bedrock 方块状态和 block entity SNBT，
输出 Java 方块 hash。返回值中的 `javaNBT` 当前为空映射。读存档时用 `Convert`，已有规范化 Bedrock hash 时用 `ConvertHashes`。
转换结果先交给 fixer 修复邻接状态，再交给 viewer；含水状态由 splitlayer 拆成方块层和水层。
`special_nemc` 场景直接构造 Bedrock 方块层、第二层水和 block entity SNBT，经 `bedrock.Convert` 转成 Java。
床颜色、花盆植物和半砖含水状态由源数据恢复，门、楼梯拐角和覆雪草方块的邻接状态交给 fixer 补齐。

- [x] **床的颜色**：从 block entity 的 `color` 染料编号生成 Java 床名，保留基础转换得到的朝向、床头尾和占用状态。
- [x] **花盆里的植物**：把 block entity 的 `PlantBlock` 转成 Java 植物名，再生成存在于 Java 属性表中的 `potted_` 方块。
- [x] **含水**：第二方块层是 `water` 或 `flowing_water` 时，输出 Java 状态附带 `waterlogged="true"`。
- **门、栅栏门、活板门的 `powered`**：表示是否被红石供能，要做红石信号传播才知道，邻居几何给不了。按
  `neomega-blocks/third_party/gen_resource/java-roundtrip` 的 `go run .` 生成的 `tmp/REPORT.md` 逐方块表，
  门为 21 种 / 672 个状态，栅栏门为 12 种 / 192 个状态，活板门为 21 种 / 336 个状态，合计 54 种 / 1200 个状态。
  统计输入是该工具枚举的 Java 状态全集。这个属性不影响外观，渲染上可以接受丢失。

## 2. NBT 方块（架构已初步具备）

判据：在 **Java 的方块表达**里，关键信息也只存在 block entity NBT 里，不在 block state 里。纯 hash 方块层需要配套的
NBT 才能保留这类外观信息。

判据看的是 Java 侧。床的颜色在 Bedrock 里存 NBT，在 Java 里编进了方块名（`red_bed`、`blue_bed`），所以它属于 1b，不属于这里。

- **旗帜的图案**：颜色在方块名里，朝向在 block state 里，图案只在 NBT 的 `patterns` 列表里。Java 图案已由 Go 合成私有贴图，姿态固定；Bedrock 图案仍需转换器输出对应的 Java NBT。
- **展示框**：在 Java 里，展示框是实体（entity），不是方块，本身就不在 LayerStorage 里，里面的物品更不在。
  Bedrock 的展示框是带 block entity 的方块，映射到 Java 时要从方块层挪到实体层。这一点凭记忆，未联网核实。

通道已经有了：输入除了 hash 层，还带一份按格子坐标索引的 SNBT，Dispatcher 把受支持的 NBT 方块交给 block actor，由它按
NBT 算出几何。每种方块是 `blockactor/` 下的一个子包，新增一种只加一个包。

- [x] 陶罐：四面纹样按 `sherds` 换贴图，NBT 结构见 `blockactor/decoratedpot/NBT.md`。
- [x] 旗帜：按 Java `patterns` 或旧 `Patterns` 合成私有贴图，经 NVFR v2 传输并绑定，见 [旗帜说明](../../neomega-viewer-core/viewer/blockactor/banner/NBT.md)。
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
- `special_nemc`：用 Bedrock 方块状态摆放，床颜色和花盆植物写在 block entity SNBT 中，半砖的水放在第二方块层。
  `neomega-converter-core/bedrock` 把这些源数据转成 Java 状态。

床颜色、花盆植物和含水状态用于检查源数据转换。门、楼梯拐角和覆雪草方块用于检查 fixer：在页面上打开修复开关后，
`special_nemc` 的方块状态与 `special_java` 逐格一致。两份配方分别见 `demo/scenes/special_nemc.go` 和 `demo/scenes/scenes.go`。

含水方块的叠加效果在 `waterlogged` 和 `water_cube` 场景里看，方块实体与特效在 `block_entities` 场景里看。
