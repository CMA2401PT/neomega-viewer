import { registerHooks } from "node:module";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";

// 每个 npm 测试从当前包解析 Three，应用和共享源码始终使用同一实例。
const parentURL = pathToFileURL(resolve("package.json")).href;
registerHooks({
  resolve(specifier, context, nextResolve) {
    return nextResolve(
      specifier,
      specifier === "three" || specifier.startsWith("three/")
        ? { ...context, parentURL }
        : context,
    );
  },
});
