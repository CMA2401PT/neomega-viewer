# 来源与许可

本项目原创的 Go 解析、HTTP 服务、TypeScript 绘制适配和网页采用 [MIT](LICENSE)。下列依赖和输入素材的许可独立保留。

## 绘制代码

浏览器使用 [Three.js](https://github.com/mrdoob/three.js)，版本由 `web/package-lock.json` 固定为 0.186.1。

`Copyright © 2010-2026 three.js authors`

Three.js 的 MIT 原文保存在 [web/public/THIRD_PARTY_NOTICES.txt](web/public/THIRD_PARTY_NOTICES.txt)，网页构建会将其复制到 `web/dist`。Vite 和 TypeScript 是构建依赖，各自保留包内许可。

## 状态与资源

子区块使用相邻 `neomega-celler` 中的 `celler/layer_storage.Palette[uint32]` 存储接口。

Java 状态全集和方块哈希使用相邻的 `neomega-blocks` preset。转换数据的准备依赖 Amulet Team 的 PyMCTranslate。本项目使用现有 Java 1.21.4 目标表；本地转换库记录的 build 317 对应上游 1.2.30。该版本采用 Amulet Team License 1.0.0，包含非商业、教育和非竞争等使用条款；它不是 MIT 许可。完整条款和来源见 [neomega-blocks 的声明](../neomega-blocks/third_party/pymctranslate-notices/NOTICE.md)。

Required Notice: Copyright Amulet Team. (https://www.amuletmc.com/)

模型、纹理和特殊方块的静态外形来自 `neomega-assets` 交付文件。其中的社区模板来自 `zardoy/mc-assets` 固定提交 `c63d6dacd358a8ea73b453d6958167b49dcc3f6c`，声明为 `Copyright 2024 Vitaly Turovsky, contributors`，采用 MIT。原始模型署名由资源生成器保留。具体来源和适配关系见 [neomega-assets/NOTICE.md](../neomega-assets/NOTICE.md)。

Minecraft 模型、纹理及其衍生素材保留原权利人的权利。资源提取、图集打包和顶点生成不产生新的素材授权。应用分发资源时，应保留 `dist/final/THIRD_PARTY_NOTICES.txt`，并遵守输入素材适用的 [Minecraft EULA](https://www.minecraft.net/eula)、[Usage Guidelines](https://www.minecraft.net/usage-guidelines) 和额外资源包条款。

服务端分发时保留本文件、项目许可证、转换数据的许可声明与配套资源声明。单独分发网页时保留 `web/dist/THIRD_PARTY_NOTICES.txt`；外部提供的模型和纹理仍按其自身来源附带声明。
