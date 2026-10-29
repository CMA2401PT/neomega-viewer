import { AxisGizmo, BlockViewport, loadAtlas } from "../render";
import {
  getJSON,
  type Binding,
  type Catalog,
  type Group,
  type Packet,
} from "./api";
import { propertyChoices } from "./states";
import "./style.css";

// This page owns selection/network state. The render exports have neither.
// The whole page is the 3D stage; every control floats over the canvas.
document.querySelector<HTMLDivElement>("#app")!.innerHTML = `
  <div id="viewport"></div>
  <div class="empty-note" id="empty-note" hidden>此方块没有可见表面</div>

  <header class="topbar">
    <div class="mark"><span class="mark-glyph"></span><span class="mark-name">Neomega</span></div>
    <nav class="view-switch" aria-label="渲染目标"><a href="/" class="active">单方块</a><a href="/subchunk.html">子区块</a></nav>
    <div class="topbar-spacer"></div>
  </header>

  <div class="view-cluster">
    <canvas class="gizmo" id="gizmo" width="64" height="64" aria-hidden="true"></canvas>
    <div class="view-seg" role="group" aria-label="投影方式"><button id="perspective" class="active">透视</button><button id="orthographic">正交</button></div>
    <div class="view-toggles">
      <button class="view-chip on" id="grid" aria-pressed="true"><span class="box"></span>地面网格</button>
      <button class="view-chip" id="frame" aria-pressed="false"><span class="box"></span>形状边界</button>
    </div>
    <button class="view-reset" id="reset">重置视角</button>
  </div>

  <aside class="panel" aria-label="方块选择与属性">
    <div class="panel-head">
      <span class="eyebrow">Single Block</span>
      <h1 id="block-title">准备预览</h1>
      <div class="meta"><span class="chip" id="version">连接中</span><span class="chip" id="catalog-count"></span><span class="status" id="status">加载中</span></div>
      <p class="hint" id="java-name">加载方块目录与纹理图集</p>
    </div>
    <div class="panel-scroll">
      <div class="section">
        <label class="search"><span>⌕</span><input id="search" type="search" placeholder="搜索名称，如 glass、oak…" aria-label="搜索方块"></label>
        <div class="quick" id="quick" aria-label="常见方块"></div>
        <div class="row-head">方块列表<span id="list-count"></span></div>
        <div class="block-list" id="block-list" role="list" aria-label="方块列表"></div>
      </div>

      <div class="section">
        <div class="field"><label for="states">存储的方块状态</label><select id="states" aria-label="存储的方块状态"></select></div>
        <div class="field"><label for="block-id">Java block hash</label><div class="id-input"><input id="block-id" inputmode="numeric" aria-label="Block ID"><button id="load-id" title="按 ID 加载">加载</button></div></div>
      </div>

      <div class="section">
        <div class="sub-head"><h3>方块属性</h3><button id="restore-properties">还原</button></div>
        <p class="hint">切换属性会选择对应状态的 Java block hash。</p>
        <div id="properties" class="properties"></div>
      </div>

      <div class="section">
        <div id="renderer-tag" class="renderer-tag">—</div>
        <p class="hint" id="preview-description">Go 查询共享几何，浏览器绘制顶点。</p>
        <details><summary>查看 Java 状态</summary><pre id="source-state"></pre></details>
      </div>

      <div class="section">
        <div class="panel-foot">列表使用 Java block hash，同一名称可包含多种状态。<br><a href="/THIRD_PARTY_NOTICES.txt" target="_blank" rel="noopener">代码许可</a> · <a href="/assets/THIRD_PARTY_NOTICES.txt" target="_blank" rel="noopener">资源来源</a></div>
      </div>
    </div>
  </aside>

  <div class="metrics">
    <div><span id="quad-count">—</span><small>四边形</small></div>
    <div><span id="triangle-count">—</span><small>三角形</small></div>
    <div><span id="batch-count">—</span><small>材质组</small></div>
    <div><span id="material-modes">—</span><small>表面类型</small></div>
  </div>
`;
const el = <T extends HTMLElement>(id: string): T =>
  document.getElementById(id) as T;
let catalog: Catalog;
let viewport: BlockViewport;
let currentGroup = "";
let currentID = 0;
let states: Binding[] = [];
let initialID = 0;
let generation = 0;
let shown = 120;

const examples = [
  ["实心", "stone"],
  ["玻璃", "glass"],
  ["半砖", "oak_slab"],
  ["栅栏", "oak_fence"],
  ["草", "short_grass"],
  ["箱子", "chest"],
  ["树叶", "oak_leaves"],
];
const labels: Record<string, string> = {
  opaque: "不透明",
  cutout: "镂空",
  blend: "半透明",
};
function status(text: string, error = false): void {
  el("status").textContent = text;
  el("status").classList.toggle("error", error);
}
function fail(error: unknown): void {
  status(error instanceof Error ? error.message : String(error), true);
}

function list(): void {
  const text = el<HTMLInputElement>("search").value.toLowerCase().trim();
  const groups = catalog.groups.filter(
    (group) =>
      group.name.includes(text) ||
      group.javaNames.some((name) => name.includes(text)),
  );
  el("list-count").textContent = `${groups.length}`;
  const root = el("block-list");
  root.replaceChildren();
  for (const group of groups.slice(0, shown)) {
    const button = document.createElement("button");
    button.className = "block-row";
    button.classList.toggle("selected", group.name === currentGroup);
    button.setAttribute("role", "listitem");
    button.title = group.name;
    const icon = document.createElement("span");
    icon.className = "cube-icon";
    const name = document.createElement("span");
    name.className = "block-name";
    name.textContent = group.name.replace("minecraft:", "");
    const count = document.createElement("span");
    count.className = "state-count";
    count.textContent = String(group.count);
    button.append(icon, name, count);
    button.onclick = () => void selectGroup(group);
    root.append(button);
  }
  if (groups.length > shown) {
    const more = document.createElement("button");
    more.className = "more";
    more.textContent = "显示更多";
    more.onclick = () => {
      shown += 120;
      list();
    };
    root.append(more);
  }
  if (groups.length === 0) {
    const note = document.createElement("p");
    note.className = "hint";
    note.textContent = "没有匹配的方块";
    root.append(note);
  }
}

async function selectGroup(
  group: Group,
  preferredID?: number,
): Promise<void> {
  const token = ++generation;
  currentGroup = group.name;
  list();
  status("读取状态…");
  try {
    const result = await getJSON<Binding[]>(
      `/api/states?name=${encodeURIComponent(group.name)}`,
    );
    if (token !== generation) return;
    states = result;
    const select = el<HTMLSelectElement>("states");
    select.replaceChildren();
    for (const binding of states) {
      const description = JSON.stringify(binding.java.properties);
      select.add(
        new Option(
          description === "{}" ? "默认状态" : description,
          String(binding.id),
        ),
      );
    }
    currentID = preferredID ?? group.exampleId;
    initialID = currentID;
    await showBlock();
  } catch (error) {
    if (token === generation) fail(error);
  }
}

async function showBlock(): Promise<void> {
  const token = ++generation;
  el<HTMLSelectElement>("states").value = String(currentID);
  status("读取几何…");
  try {
    const packet = await getJSON<Packet>("/api/block", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        id: currentID,
      }),
    });
    if (token !== generation) return;
    viewport.set(packet.geometry);
    el("block-title").textContent = packet.binding.java.name.replace(
      "minecraft:",
      "",
    );
    el("java-name").textContent = packet.binding.java.name;
    el<HTMLInputElement>("block-id").value = String(packet.binding.id);
    el("empty-note").textContent = "此方块没有可见表面";
    el("empty-note").hidden = packet.geometry.quads !== 0;
    el("quad-count").textContent = String(packet.geometry.quads);
    el("triangle-count").textContent = String(packet.geometry.quads * 2);
    el("batch-count").textContent = String(packet.geometry.batches.length);
    el("material-modes").textContent =
      [
        ...new Set(
          packet.geometry.batches.map((batch) => labels[batch.surface.mode]),
        ),
      ].join(" / ") || "无表面";
    const family = packet.renderer.family;
    el("renderer-tag").textContent =
      packet.renderer.kind === "special"
        ? `静态模型 · ${family}`
        : packet.renderer.kind === "fluid"
          ? "静止流体预览"
          : packet.renderer.kind === "invisible"
            ? "不可见方块"
            : "方块模型";
    el("preview-description").textContent =
      packet.renderer.kind === "special"
        ? "使用固定外形展示，不包含动画或实时外观。"
        : "按当前 Java block hash 查询已生成的几何。";
    el("source-state").textContent = JSON.stringify(packet.binding.java, null, 2);
    renderProperties(packet.binding);
    status("预览就绪");
    document.title = `${packet.binding.java.name.replace("minecraft:", "")} · Neomega`;
  } catch (error) {
    if (token === generation) {
      viewport.set({
        batches: [],
        bounds: { min: [0, 0, 0], max: [1, 1, 1] },
        quads: 0,
      });
      const binding = states.find((binding) => binding.id === currentID);
      el("block-title").textContent =
        binding?.java.name.replace("minecraft:", "") ?? String(currentID);
      el("java-name").textContent = binding?.java.name ?? "";
      el<HTMLInputElement>("block-id").value = String(currentID);
      el("empty-note").textContent = "无法生成此预览，请查看上方错误信息";
      el("empty-note").hidden = false;
      for (const id of ["quad-count", "triangle-count", "batch-count"])
        el(id).textContent = "0";
      el("material-modes").textContent = "无可用几何";
      el("renderer-tag").textContent = "预览不可用";
      el("properties").replaceChildren();
      el("source-state").textContent = binding
        ? JSON.stringify(binding.java, null, 2)
        : "";
      el("preview-description").textContent = "请检查错误信息和当前预览属性。";
      fail(error);
    }
  }
}

function renderProperties(current: Binding): void {
  const root = el("properties");
  root.replaceChildren();
  for (const [name, values] of Object.entries(propertyChoices(states, current)).sort(
    ([a], [b]) => a.localeCompare(b),
  )) {
    if (values.length < 2) continue;
    const label = document.createElement("label");
    label.className = "property";
    const title = document.createElement("span");
    title.textContent = name;
    const select = document.createElement("select");
    select.setAttribute("aria-label", `Java 属性 ${name}`);
    values.forEach((binding) =>
      select.add(new Option(binding.java.properties[name], String(binding.id))),
    );
    select.value = String(current.id);
    select.onchange = () => {
      currentID = Number(select.value);
      void showBlock();
    };
    label.append(title, select);
    root.append(label);
  }
  if (!root.children.length) {
    const p = document.createElement("p");
    p.className = "hint no-properties";
    p.textContent = "此方块没有可切换的属性。";
    root.append(p);
  }
}

async function start(): Promise<void> {
  catalog = await getJSON<Catalog>("/api/blocks");
  const texture = await loadAtlas(catalog.atlasUrl);
  viewport = new BlockViewport(el("viewport"), texture);
  const gizmo = new AxisGizmo(el<HTMLCanvasElement>("gizmo"));
  viewport.observeCamera((q) => gizmo.update(q));
  el("version").textContent = `JAVA ${catalog.version}`;
  el("catalog-count").textContent =
    `${catalog.groups.length.toLocaleString()} 类`;
  for (const [label, name] of examples) {
    const button = document.createElement("button");
    button.textContent = label;
    button.onclick = () => {
      const target = `minecraft:${name}`;
      const group =
        catalog.groups.find(
          (group) => group.name === target && group.javaNames.includes(target),
        ) ?? catalog.groups.find((group) => group.javaNames.includes(target));
      if (group) {
        el<HTMLInputElement>("search").value = "";
        void selectGroup(group);
      }
    };
    el("quick").append(button);
  }
  el<HTMLInputElement>("search").oninput = () => {
    shown = 120;
    list();
  };
  el<HTMLSelectElement>("states").onchange = () => {
    currentID = Number(el<HTMLSelectElement>("states").value);
    initialID = currentID;
    void showBlock();
  };
  el("restore-properties").onclick = () => {
    currentID = initialID;
    void showBlock();
  };
  el("reset").onclick = () => viewport.reset();
  // Grid/frame are transparent toggle chips, not form checkboxes.
  const toggleChip = (id: string, apply: (on: boolean) => void): void => {
    const chip = el<HTMLButtonElement>(id);
    chip.onclick = () => {
      const on = !chip.classList.contains("on");
      chip.classList.toggle("on", on);
      chip.setAttribute("aria-pressed", String(on));
      apply(on);
    };
  };
  toggleChip("grid", (on) => viewport.showGrid(on));
  toggleChip("frame", (on) => viewport.showFrame(on));
  for (const [id, ortho] of [
    ["perspective", false],
    ["orthographic", true],
  ] as const) {
    el(id).onclick = () => {
      viewport.projection(ortho);
      el("perspective").classList.toggle("active", !ortho);
      el("orthographic").classList.toggle("active", ortho);
    };
  }
  el("load-id").onclick = async () => {
    const token = ++generation;
    try {
      const binding = await getJSON<Binding>(
        `/api/state?id=${encodeURIComponent(el<HTMLInputElement>("block-id").value)}`,
      );
      if (token !== generation) return;
      const group = catalog.groups.find(
        (group) => group.name === binding.java.name,
      );
      if (group) await selectGroup(group, binding.id);
    } catch (error) {
      if (token === generation) fail(error);
    }
  };
  const stone =
    catalog.groups.find((group) =>
      group.javaNames.includes("minecraft:stone"),
    ) ?? catalog.groups[0];
  await selectGroup(stone);
  window.addEventListener(
    "pagehide",
    () => {
      viewport.dispose();
      gizmo.dispose();
      texture.dispose();
    },
    { once: true },
  );
}
void start().catch(fail);
