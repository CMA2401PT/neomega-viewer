import { registerHooks } from "node:module";

// 共享绘制源码使用本前端工程的 Three 依赖。
registerHooks({
  resolve(specifier, context, nextResolve) {
    return nextResolve(
      specifier,
      specifier === "three" || specifier.startsWith("three/")
        ? { ...context, parentURL: import.meta.url }
        : context,
    );
  },
});
