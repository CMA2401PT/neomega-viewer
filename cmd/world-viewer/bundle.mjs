// 构建应用页面；贴图与几何由 viewer-core 的资源生成入口统一维护。
import { mkdirSync, copyFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { resolve, join } from "node:path";
import { execFileSync } from "node:child_process";

const root = fileURLToPath(new URL(".", import.meta.url));
const frontend = join(root, "frontend");
execFileSync("npm", ["ci"], { cwd: frontend, stdio: "inherit" });
mkdirSync(join(frontend, "public"), { recursive: true });
copyFileSync(join(root, "../../LICENSE"), join(frontend, "public/LICENSE.txt"));
copyFileSync(
  join(frontend, "node_modules/three/LICENSE"),
  join(frontend, "public/THREE-LICENSE.txt"),
);
execFileSync("npm", ["run", "build"], { cwd: frontend, stdio: "inherit" });
