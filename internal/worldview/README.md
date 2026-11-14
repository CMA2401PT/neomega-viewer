# 存档浏览服务组装

这个包把存档来源、精细网格和体素区域接成可切换的 HTTP 浏览服务。它持有当前存档的一代内容，先发布新存档，再取消并等待旧任务，最后关闭旧来源。命令行入口只提供配置和进程生命周期。

`OpenSource` 负责打开 Java、Bedrock、PackFS 或结构文件，并准备初始落点与维度描述。Java 与 Bedrock 的存储适配仍放在 `source` 包。没有指定存档时，来源借用 `internal/startscene` 提供的只读方块和取景数据。

有了来源和资源表，用 `NewPipeline` 准备精细网格构建器，再交给 `NewWorlds(source, pipeline, version)` 管理存档代次。这里会接管来源的关闭责任。`Handler(ServerOptions)` 提供页面、文件浏览和存档切换路由，并复用 `viewhttp` 的列构建调度及 `minimap` 的区域服务。文件浏览委托给 `internal/filebrowse`，目录条目不需要持有存档句柄。

每个请求只取得一次当前内容，避免把两代存档的数据拼起来。切换时，三维列构建和体素区域均退役后才关闭来源；Bedrock 的底层读取还要经过自己的关闭屏障。来源打开失败时保持旧内容可用。HTTP 路由、取消与缓存隔离的测试和组装代码放在同一包，便于检查这一条完整的生命周期。

改动运行参数和系统浏览器启动时，查看 `cmd/world-viewer` 与 `internal/desktop`。改动来源适配、网格编码或请求缓存时，查看 `source`、`column`、`viewhttp`。这些包不依赖本包，因此演示和其他应用也可以分别组合它们。
