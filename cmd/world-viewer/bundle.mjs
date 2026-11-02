// Run via go generate. All runtime files are copied into the embedded bundle.
import { mkdirSync, copyFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { resolve, join } from "node:path";
import { execFileSync } from "node:child_process";

const root = fileURLToPath(new URL(".", import.meta.url));
const assets = resolve(root, process.argv[2] ?? "../../../neomega-assets/dist");
const resources = join(root, "../../viewassets/bundle/resources");
mkdirSync(join(resources, "entity"), { recursive: true });
for (const [source, destination] of [
  ["final/blocksAtlases.json", "blocksAtlases.json"],
  ["final/blocksAtlasLatest.png", "atlas.png"],
  ["final/THIRD_PARTY_NOTICES.txt", "THIRD_PARTY_NOTICES.txt"],
  [
    "works/04-renderers/assets/minecraft/textures/entity/enchanting_table_book.png",
    "entity/enchanting_table_book.png",
  ],
])
  copyFileSync(join(assets, source), join(resources, destination));
const frontend = join(root, "frontend");
execFileSync("npm", ["ci"], { cwd: frontend, stdio: "inherit" });
mkdirSync(join(frontend, "public"), { recursive: true });
copyFileSync(join(root, "../../LICENSE"), join(frontend, "public/LICENSE.txt"));
copyFileSync(
  join(frontend, "node_modules/three/LICENSE"),
  join(frontend, "public/THREE-LICENSE.txt"),
);
execFileSync("npm", ["run", "build"], { cwd: frontend, stdio: "inherit" });
