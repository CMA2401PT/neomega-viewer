import { registerHooks } from "node:module";

// 核心沿用 demo 的依赖，测试从同一位置解析 Three。
registerHooks({
  resolve(specifier, context, nextResolve) {
    return nextResolve(specifier,
      specifier === "three" || specifier.startsWith("three/")
        ? { ...context, parentURL: import.meta.url }
        : context,
    );
  },
});
