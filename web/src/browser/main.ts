import { BlockViewport, loadAtlas } from "../render";
import {
  getJSON,
  type Binding,
  type Catalog,
  type Group,
  type Packet,
} from "./api";
import "./style.css";

// This page owns selection/network state. The render exports have neither.
document.querySelector<HTMLDivElement>("#app")!.innerHTML = `
  <header class="topbar">
    <a class="brand" href="/" aria-label="Neomega 首页"><span class="brandmark">N</span><span>NEOMEGA<span class="brand-sub">BLOCK EXPLORER</span></span></a>
    <span class="version" id="version">正在连接…</span>
    <nav class="view-switch" aria-label="渲染目标"><a href="/" class="active">单方块</a><a href="/subchunk.html">子区块</a></nav>
    <span class="static-badge"><i></i>静态方块浏览器</span>
  </header>
  <main class="workspace">
    <aside class="sidebar">
      <div class="section-heading"><span class="eyebrow">LIBRARY</span><span class="muted" id="catalog-count"></span></div>
      <h1>选择一个方块</h1><p class="intro">从状态到形状，逐个查看。</p>
      <label class="search"><span>⌕</span><input id="search" type="search" placeholder="搜索名称，如 glass、oak…" aria-label="搜索方块"></label>
      <div class="quick" id="quick" aria-label="常见方块"></div>
      <div class="list-heading"><span>方块列表</span><span id="list-count"></span></div>
      <div class="block-list" id="block-list" role="list" aria-label="方块列表"></div>
      <div class="sidebar-foot">列表使用 Java block hash。<br>同一名称可包含多种状态。<br><a href="/THIRD_PARTY_NOTICES.txt" target="_blank" rel="noopener">代码许可</a> · <a href="/assets/THIRD_PARTY_NOTICES.txt" target="_blank" rel="noopener">资源来源</a></div>
    </aside>
    <section class="stage">
      <div class="stage-heading"><div><span class="eyebrow">SINGLE BLOCK</span><h2 id="block-title">准备预览</h2><p id="java-name" class="muted">加载方块目录与纹理图集</p></div><span id="status" class="status">加载中</span></div>
      <div class="viewer-shell"><div id="viewport"></div><div class="canvas-note" id="empty-note" hidden>此方块没有可见表面</div><div class="canvas-caption"><span class="axis"><b>X</b><b>Y</b><b>Z</b></span><span>拖动旋转 · 滚轮缩放 · 右键平移</span></div></div>
      <div class="toolbar"><div class="segments"><button id="perspective" class="active">透视</button><button id="orthographic">正交</button></div><label><input id="grid" type="checkbox" checked>地面网格</label><label><input id="frame" type="checkbox">形状边界</label><button id="reset" class="reset">↺ 重置视角</button></div>
      <div class="metrics"><div><span id="quad-count">—</span><small>四边形</small></div><div><span id="triangle-count">—</span><small>三角形</small></div><div><span id="batch-count">—</span><small>材质组</small></div><div><span id="material-modes">—</span><small>表面类型</small></div></div>
    </section>
    <aside class="inspector">
      <div class="section-heading"><span class="eyebrow">INSPECTOR</span><span class="small-dot"></span></div><h2>状态与外观</h2>
      <div class="field"><label for="states">存储的方块状态</label><select id="states" aria-label="存储的方块状态"></select></div>
      <div class="field"><label for="block-id">Java block hash</label><div class="id-input"><input id="block-id" inputmode="numeric" aria-label="Block ID"><button id="load-id" title="按 ID 加载">加载</button></div></div>
      <div class="divider"></div><div class="property-heading"><h3>静态预览属性</h3><button id="restore-properties">还原</button></div><p class="hint">只调整当前预览，不改变存储的 ID。</p><div id="properties" class="properties"></div>
      <div class="field seed-field"><label for="seed">模型选择种子</label><input id="seed" type="number" value="0" min="0" max="4294967295" step="1"></div>
      <div class="divider"></div><div class="mapping-label">解析结果</div><div id="renderer-tag" class="renderer-tag">—</div><p class="hint" id="preview-description">Go 解析状态与模型，浏览器绘制顶点。</p>
      <details><summary>查看来源与模型</summary><h4>Java 原始状态</h4><pre id="source-state"></pre><h4>预览状态</h4><pre id="target-state"></pre><h4>选中的模型</h4><pre id="models"></pre></details>
    </aside>
  </main>
`;
const el = <T extends HTMLElement>(id: string): T =>
  document.getElementById(id) as T;
let catalog: Catalog;
let viewport: BlockViewport;
let currentGroup = "";
let currentID = 0;
let states: Binding[] = [];
let overrides: Record<string, string> = {};
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
    icon.textContent = "◇";
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
  preferredJava?: string,
  preferredID?: number,
): Promise<void> {
  const token = ++generation;
  currentGroup = group.name;
  overrides = {};
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
    currentID =
      states.find((binding) =>
        preferredID !== undefined
          ? binding.id === preferredID
          : binding.java.name === preferredJava,
      )?.id ?? group.exampleId;
    select.value = String(currentID);
    await showBlock();
  } catch (error) {
    if (token === generation) fail(error);
  }
}

async function showBlock(): Promise<void> {
  const token = ++generation;
  status("生成几何…");
  try {
    const packet = await getJSON<Packet>("/api/block", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        id: currentID,
        seed: Number(el<HTMLInputElement>("seed").value),
        properties: overrides,
      }),
    });
    if (token !== generation) return;
    viewport.set(packet.geometry);
    el("block-title").textContent = packet.state.name.replace("minecraft:", "");
    el("java-name").textContent = packet.state.name;
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
        : "状态、模型和纹理引用均在 Go 中解析。";
    el("source-state").textContent = JSON.stringify(packet.binding.java, null, 2);
    el("target-state").textContent = JSON.stringify(packet.state, null, 2);
    el("models").textContent =
      packet.applications.map((application) => application.model).join("\n") ||
      "由静态渲染分类处理";
    renderProperties(packet);
    status("预览就绪");
    document.title = `${packet.state.name.replace("minecraft:", "")} · Neomega`;
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
      el("target-state").textContent = binding
        ? JSON.stringify(binding.java, null, 2)
        : "";
      el("models").textContent = "";
      el("preview-description").textContent = "请检查错误信息和当前预览属性。";
      fail(error);
    }
  }
}

function renderProperties(packet: Packet): void {
  const root = el("properties");
  root.replaceChildren();
  for (const [name, values] of Object.entries(packet.propertyValues).sort(
    ([a], [b]) => a.localeCompare(b),
  )) {
    if (values.length < 2) continue;
    const label = document.createElement("label");
    label.className = "property";
    const title = document.createElement("span");
    title.textContent = name;
    const select = document.createElement("select");
    select.setAttribute("aria-label", `Java 属性 ${name}`);
    values.forEach((value) => select.add(new Option(value, value)));
    select.value = packet.state.properties[name];
    select.onchange = () => {
      overrides[name] = select.value;
      void showBlock();
    };
    label.append(title, select);
    root.append(label);
  }
  if (!root.children.length) {
    const p = document.createElement("p");
    p.className = "hint no-properties";
    p.textContent = "此模型没有可切换的外观属性。";
    root.append(p);
  }
}

async function start(): Promise<void> {
  catalog = await getJSON<Catalog>("/api/blocks");
  const texture = await loadAtlas(catalog.atlasUrl);
  viewport = new BlockViewport(el("viewport"), texture);
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
        void selectGroup(group, target);
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
    overrides = {};
    void showBlock();
  };
  el<HTMLInputElement>("seed").onchange = () => void showBlock();
  el("restore-properties").onclick = () => {
    overrides = {};
    void showBlock();
  };
  el("reset").onclick = () => viewport.reset();
  el<HTMLInputElement>("grid").onchange = () =>
    viewport.showGrid(el<HTMLInputElement>("grid").checked);
  el<HTMLInputElement>("frame").onchange = () =>
    viewport.showFrame(el<HTMLInputElement>("frame").checked);
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
      if (group) await selectGroup(group, undefined, binding.id);
    } catch (error) {
      if (token === generation) fail(error);
    }
  };
  const stone =
    catalog.groups.find((group) =>
      group.javaNames.includes("minecraft:stone"),
    ) ?? catalog.groups[0];
  await selectGroup(stone, "minecraft:stone");
  window.addEventListener(
    "pagehide",
    () => {
      viewport.dispose();
      texture.dispose();
    },
    { once: true },
  );
}
void start().catch(fail);
