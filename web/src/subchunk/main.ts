import { MeshViewport, decodeMesh, loadAtlas } from "../render";
import { getJSON } from "../browser/api";
import "../browser/style.css";
import "./style.css";

interface Scene {
  id: string;
  title: string;
  description: string;
}
interface Catalog {
  version: string;
  atlasUrl: string;
  scenes: Scene[];
}
interface Stats {
  inputQuads: number;
  removedQuads: number;
  clippedQuads: number;
  visibleQuads: number;
  outputQuads: number;
  blocks: number;
  bakedShapes: number;
}

const el = <T extends HTMLElement>(id: string): T =>
  document.getElementById(id) as T;
document.querySelector<HTMLDivElement>("#app")!.innerHTML = `
  <header class="topbar">
    <a class="brand" href="/"><span class="brandmark">N</span><span>NEOMEGA<span class="brand-sub">GEOMETRY EXPLORER</span></span></a>
    <span class="version" id="version">正在连接…</span>
    <nav class="view-switch" aria-label="渲染目标"><a href="/">单方块</a><a href="/subchunk.html" class="active">子区块</a></nav>
    <span class="static-badge"><i></i>静态子区块浏览器</span>
  </header>
  <main class="workspace">
    <aside class="sidebar">
      <div class="section-heading"><span class="eyebrow">SUBCHUNK LAB</span><span class="muted">16 × 16 × 16</span></div>
      <h1>观察面与形状</h1><p class="intro">选择一个场景，比较合并前后的网格。</p>
      <div id="scenes" class="scene-list"></div>
      <div class="upload"><label for="layer-file">加载自己的子区块</label><p class="hint">4096 个小端 uint32 Java block hash，顺序为 X-Z-Y。</p><input id="layer-file" type="file" accept=".nvblocks,.bin"></div>
      <div class="sidebar-foot">场景使用 Java block hash。<br><a href="/THIRD_PARTY_NOTICES.txt" target="_blank" rel="noopener">代码许可</a> · <a href="/assets/THIRD_PARTY_NOTICES.txt" target="_blank" rel="noopener">资源来源</a></div>
    </aside>
    <section class="stage">
      <div class="stage-heading"><div><span class="eyebrow">ISOLATED SUBCHUNK</span><h2 id="scene-title">准备预览</h2><p id="scene-caption" class="muted">加载静态资源</p></div><span id="status" class="status">加载中</span></div>
      <div class="viewer-shell"><div id="viewport"></div><div id="empty-note" class="canvas-note" hidden>此子区块没有可见表面</div><div class="canvas-caption"><span class="axis"><b>X</b><b>Y</b><b>Z</b></span><span>拖动旋转 · 滚轮缩放 · 右键平移</span></div></div>
      <div class="toolbar"><div class="segments"><button id="perspective" class="active">透视</button><button id="orthographic">正交</button></div><label><input id="wireframe" type="checkbox">三角网格</label><label><input id="frame" type="checkbox">形状边界</label><label><input id="grid" type="checkbox" checked>地面网格</label><button id="reset" class="reset">↺ 重置视角</button></div>
      <div class="metrics"><div><span id="quad-count">—</span><small>输出四边形</small></div><div><span id="triangle-count">—</span><small>三角形</small></div><div><span id="batch-count">—</span><small>材质组</small></div><div><span id="wire-size">—</span><small>二进制响应</small></div></div>
    </section>
    <aside class="inspector">
      <div class="section-heading"><span class="eyebrow">MESH PIPELINE</span><span class="small-dot"></span></div><h2>比较处理结果</h2>
      <div class="mesh-options">
        <label><input id="cull" type="checkbox" checked><span>剔除遮挡面<small>保留半砖旁没有被盖住的部分</small></span></label>
        <label><input id="greedy" type="checkbox" checked><span>合并相邻面<small>兼容的材质与 UV 才能合并</small></span></label>
        <label><input id="vary" type="checkbox"><span>按位置变化模型<small>保留候选模型的旋转与镜像差异</small></span></label>
      </div>
      <div class="field seed-field"><label for="seed">模型选择种子</label><input id="seed" type="number" value="0" min="0" max="4294967295" step="1"></div>
      <div class="divider"></div><h3 class="pipeline-title">四边形的变化</h3>
      <div class="pipeline"><div><label>模型原始面<span id="input-quads">—</span></label><i id="bar-input"></i></div><div><label>剔除后<span id="visible-quads">—</span></label><i id="bar-visible"></i></div><div><label>合并后<span id="output-quads">—</span></label><i id="bar-output"></i></div></div>
      <p class="hint" id="shape-stats"></p><p class="hint">不同颜色的玻璃保留界面。半透明面单独排序；树叶的镂空不会遮掉后面的物体。</p>
      <div class="divider"></div><button id="save-mesh" class="download" disabled>下载当前网格 .nvmesh</button><a id="save-layer" class="download" href="#">下载方块层 .nvblocks</a>
    </aside>
  </main>`;

let viewport: MeshViewport;
let current: Scene;
let uploaded: ArrayBuffer | undefined;
let latest: ArrayBuffer | undefined;
let generation = 0;

function status(message: string, error = false): void {
  el("status").textContent = message;
  el("status").classList.toggle("error", error);
}
function fail(error: unknown): void {
  status(error instanceof Error ? error.message : String(error), true);
}
function checked(id: string): boolean {
  return el<HTMLInputElement>(id).checked;
}
function download(data: ArrayBuffer, name: string): void {
  const url = URL.createObjectURL(
    new Blob([data], { type: "application/octet-stream" }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download = name;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

async function load(resetView = false): Promise<void> {
  const token = ++generation;
  status("解析子区块…");
  el<HTMLButtonElement>("save-mesh").disabled = true;
  const query = new URLSearchParams({
    cull: String(checked("cull")),
    greedy: String(checked("greedy")),
    vary: String(checked("vary")),
    seed: el<HTMLInputElement>("seed").value,
  });
  try {
    const response = await fetch(
      uploaded
        ? `/api/subchunk?${query}`
        : `/api/subchunks/${current.id}/mesh?${query}`,
      uploaded
        ? {
            method: "POST",
            headers: { "Content-Type": "application/octet-stream" },
            body: uploaded,
          }
        : undefined,
    );
    if (!response.ok) {
      const body = (await response.json()) as { error: string };
      throw new Error(body.error);
    }
    const data = await response.arrayBuffer();
    if (token !== generation) return;
    const mesh = decodeMesh(data);
    const stats: Stats = JSON.parse(
      response.headers.get("X-Neomega-Mesh-Stats")!,
    );
    viewport.set(mesh, { resetView });
    viewport.showWireframe(checked("wireframe"));
    latest = data;
    el("scene-title").textContent = current.title;
    el("scene-caption").textContent = current.description;
    el("empty-note").textContent = "此子区块没有可见表面";
    el("empty-note").hidden = mesh.quads !== 0;
    el("quad-count").textContent = mesh.quads.toLocaleString();
    el("triangle-count").textContent = (mesh.quads * 2).toLocaleString();
    el("batch-count").textContent = String(mesh.batches.length);
    el("wire-size").textContent = `${(data.byteLength / 1024).toFixed(1)} KiB`;
    for (const [name, value] of [
      ["input", stats.inputQuads],
      ["visible", stats.visibleQuads],
      ["output", stats.outputQuads],
    ] as const) {
      el(`${name}-quads`).textContent = value.toLocaleString();
      el(`bar-${name}`).style.width =
        `${Math.max(1, (value / Math.max(1, stats.inputQuads)) * 100)}%`;
    }
    el("shape-stats").textContent =
      `${stats.blocks.toLocaleString()} 个参与构建的方块 · ${stats.removedQuads.toLocaleString()} 个面完全移除 · ${stats.clippedQuads.toLocaleString()} 个面部分裁切`;
    el<HTMLButtonElement>("save-mesh").disabled = false;
    el<HTMLAnchorElement>("save-layer").href = uploaded
      ? "#"
      : `/api/subchunks/${current.id}/blocks`;
    status("预览就绪");
  } catch (error) {
    if (token !== generation) return;
    latest = undefined;
    viewport.set({
      batches: [],
      bounds: { min: [0, 0, 0], max: [16, 16, 16] },
      quads: 0,
    });
    el("scene-title").textContent = current.title;
    el("scene-caption").textContent = current.description;
    el("empty-note").textContent = "无法生成此预览，请查看上方错误信息";
    el("empty-note").hidden = false;
    for (const id of [
      "quad-count",
      "triangle-count",
      "batch-count",
      "wire-size",
      "input-quads",
      "visible-quads",
      "output-quads",
    ])
      el(id).textContent = "—";
    el("shape-stats").textContent = "";
    fail(error);
  }
}

async function start(): Promise<void> {
  const catalog = await getJSON<Catalog>("/api/subchunks");
  const atlas = await loadAtlas(catalog.atlasUrl);
  viewport = new MeshViewport(el("viewport"), atlas, {
    gridSize: 32,
    label: "独立子区块三维预览，可拖动旋转和滚轮缩放",
  });
  el("version").textContent = `JAVA ${catalog.version}`;
  for (const scene of catalog.scenes) {
    const button = document.createElement("button");
    button.className = "scene-card";
    button.dataset.scene = scene.id;
    const title = document.createElement("strong");
    title.textContent = scene.title;
    const description = document.createElement("span");
    description.textContent = scene.description;
    button.append(title, description);
    button.onclick = () => {
      uploaded = undefined;
      current = scene;
      el<HTMLInputElement>("layer-file").value = "";
      document
        .querySelectorAll(".scene-card")
        .forEach((node) => node.classList.toggle("selected", node === button));
      void load(true);
    };
    el("scenes").append(button);
  }
  for (const id of ["cull", "greedy", "vary", "seed"])
    el<HTMLInputElement>(id).onchange = () => void load();
  el<HTMLInputElement>("wireframe").onchange = () =>
    viewport.showWireframe(checked("wireframe"));
  el<HTMLInputElement>("frame").onchange = () =>
    viewport.showFrame(checked("frame"));
  el<HTMLInputElement>("grid").onchange = () =>
    viewport.showGrid(checked("grid"));
  el("reset").onclick = () => viewport.reset();
  for (const [id, orthographic] of [
    ["perspective", false],
    ["orthographic", true],
  ] as const) {
    el(id).onclick = () => {
      viewport.projection(orthographic);
      el("perspective").classList.toggle("active", !orthographic);
      el("orthographic").classList.toggle("active", orthographic);
    };
  }
  el("save-mesh").onclick = () => {
    if (latest) download(latest, `${current.id}.nvmesh`);
  };
  el("save-layer").onclick = (event) => {
    if (uploaded) {
      event.preventDefault();
      download(uploaded, "uploaded.nvblocks");
    }
  };
  el<HTMLInputElement>("layer-file").onchange = async () => {
    const file = el<HTMLInputElement>("layer-file").files?.[0];
    if (!file) return;
    const token = ++generation;
    const data = await file.arrayBuffer();
    if (token !== generation) return;
    uploaded = data;
    current = {
      id: "uploaded",
      title: file.name,
      description: "本地上传的 16³ 方块层",
    };
    document
      .querySelectorAll(".scene-card")
      .forEach((node) => node.classList.remove("selected"));
    await load(true);
  };
  current = catalog.scenes[0];
  document.querySelector(".scene-card")?.classList.add("selected");
  await load(true);
  window.addEventListener(
    "pagehide",
    () => {
      viewport.dispose();
      atlas.dispose();
    },
    { once: true },
  );
}
void start().catch(fail);
