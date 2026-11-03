import {
  AxisGizmo,
  MeshViewport,
  decodeFrame,
  loadAtlas,
  createEffectRegistry,
  enchantingTableBookTextureURL,
} from "@neomega/viewer/render";
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
  shapes: number;
}

const el = <T extends HTMLElement>(id: string): T =>
  document.getElementById(id) as T;
// The whole page is the 3D stage; controls float over the full-bleed canvas.
document.querySelector<HTMLDivElement>("#app")!.innerHTML = `
  <div id="viewport"></div>
  <div class="empty-note" id="empty-note" hidden>此子区块没有可见表面</div>

  <div class="view-cluster">
    <canvas class="gizmo" id="gizmo" width="64" height="64" aria-hidden="true"></canvas>
    <div class="view-controls">
    <div class="view-seg" role="group" aria-label="投影方式"><button id="perspective" class="active">透视</button><button id="orthographic">正交</button></div>
    <div class="view-toggles">
      <button class="view-chip" id="wireframe" aria-pressed="false"><span class="box"></span>三角网格</button>
      <button class="view-chip on" id="grid" aria-pressed="true"><span class="box"></span>地面网格</button>
      <button class="view-chip" id="frame" aria-pressed="false"><span class="box"></span>形状边界</button>
    </div>
    <button class="view-reset" id="reset">重置视角</button>
    </div>
  </div>

  <button class="panel-open" id="panel-open" aria-label="展开面板" hidden>子区块</button>

  <aside class="panel" id="panel" aria-label="场景与网格流水线">
    <div class="panel-head">
      <nav class="view-switch" aria-label="渲染目标"><a href="/">单方块</a><a href="/subchunk.html" class="active">子区块</a><button class="panel-collapse" id="panel-collapse" aria-label="收起面板">⟨</button></nav>
      <h1 id="scene-title">准备预览</h1>
      <div class="meta"><span class="chip" id="version">连接中</span><span class="chip">16³</span><span class="status" id="status">加载中</span></div>
    </div>

    <div class="panel-scroll">
      <div class="section">
        <div class="row-head">场景</div>
        <div id="scenes" class="scene-list"></div>
      </div>

      <div class="section">
        <div class="row-head">加载自己的子区块</div>
        <div class="upload">
          <input id="layer-file" type="file" accept=".nvblocks,.bin">
          <p class="hint">4096 个小端 uint32 Java block hash，顺序为 X-Z-Y。</p>
        </div>
      </div>

      <div class="section">
        <div class="row-head">网格流水线</div>
        <div class="mesh-options">
          <label class="toggle"><input id="fix" type="checkbox"><span class="box"></span><span class="copy"><b>修复方块状态 / 连接</b><small>补全栅栏与相邻栅栏、实心方块的连接</small></span></label>
          <label class="toggle"><input id="cull" type="checkbox" checked><span class="box"></span><span class="copy"><b>剔除遮挡面</b><small>保留半砖旁没有被盖住的部分</small></span></label>
          <label class="toggle"><input id="greedy" type="checkbox" checked><span class="box"></span><span class="copy"><b>合并相邻面</b><small>兼容的材质与 UV 才能合并</small></span></label>
        </div>
      </div>

      <div class="section">
        <h3 class="pipeline-title">四边形的变化</h3>
        <div class="pipeline">
          <div><label>模型原始面<span id="input-quads">—</span></label><span class="bar"><i id="bar-input"></i></span></div>
          <div><label>剔除后<span id="visible-quads">—</span></label><span class="bar"><i id="bar-visible"></i></span></div>
          <div><label>合并后<span id="output-quads">—</span></label><span class="bar"><i id="bar-output"></i></span></div>
        </div>
        <p class="hint" id="shape-stats"></p>
      </div>

      <div class="section">
        <button id="save-mesh" class="download" disabled>下载当前快照 .nvframe</button>
        <a id="save-layer" class="download" href="#">下载方块层 .nvblocks</a>
      </div>

      <div class="section">
        <div class="panel-foot">场景使用 Java block hash。<br><a href="/THIRD_PARTY_NOTICES.txt" target="_blank" rel="noopener">代码许可</a> · <a href="/assets/THIRD_PARTY_NOTICES.txt" target="_blank" rel="noopener">资源来源</a></div>
      </div>
    </div>
  </aside>

  <div class="metrics">
    <div><span id="quad-count">—</span><small>输出四边形</small></div>
    <div><span id="triangle-count">—</span><small>三角形</small></div>
    <div><span id="batch-count">—</span><small>材质组</small></div>
    <div><span id="wire-size">—</span><small>二进制响应</small></div>
  </div>`;

let viewport: MeshViewport;
let current: Scene;
let uploaded: ArrayBuffer | undefined;
let latest: ArrayBuffer | undefined;
let generation = 0;
// wireframe/grid/frame are view chips, not form controls; track their state.
let wireframe = false;

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
    fix: checked("fix") ? "1" : "0",
    cull: String(checked("cull")),
    greedy: String(checked("greedy")),
  });
  try {
    const response = await fetch(
      uploaded
        ? `/api/subchunk/frame?${query}`
        : `/api/subchunks/${current.id}/frame?${query}`,
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
    const frame = decodeFrame(data);
    const meshes = [frame.mesh, ...frame.actors.map((actor) => actor.mesh)];
    const actorQuads = frame.actors.reduce(
      (sum, actor) => sum + actor.mesh.quads,
      0,
    );
    const quads = frame.mesh.quads + actorQuads;
    const batches = meshes.reduce((sum, mesh) => sum + mesh.batches.length, 0);
    const stats: Stats = JSON.parse(
      response.headers.get("X-Neomega-Mesh-Stats")!,
    );
    viewport.setFrame(frame, { resetView });
    viewport.showWireframe(wireframe);
    latest = data;
    el("scene-title").textContent = current.title;
    el("empty-note").textContent = "此子区块没有可见表面";
    el("empty-note").hidden = quads !== 0 || frame.effects.length !== 0;
    el("quad-count").textContent = quads.toLocaleString();
    el("triangle-count").textContent = (quads * 2).toLocaleString();
    el("batch-count").textContent = String(batches);
    el("wire-size").textContent = `${(data.byteLength / 1024).toFixed(1)} KiB`;
    for (const [name, value] of [
      ["input", stats.inputQuads + actorQuads],
      ["visible", stats.visibleQuads + actorQuads],
      ["output", quads],
    ] as const) {
      el(`${name}-quads`).textContent = value.toLocaleString();
      el(`bar-${name}`).style.width =
        `${Math.max(1, (value / Math.max(1, stats.inputQuads + actorQuads)) * 100)}%`;
    }
    el("shape-stats").textContent =
      `${stats.blocks.toLocaleString()} 个参与静态构建的方块 · ${stats.removedQuads.toLocaleString()} 个面完全移除 · ${stats.clippedQuads.toLocaleString()} 个面部分裁切 · ${frame.actors.length} 个 actor · ${frame.effects.length} 个特效`;
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
  const [atlas, bookTexture] = await Promise.all([
    loadAtlas(catalog.atlasUrl),
    loadAtlas(enchantingTableBookTextureURL),
  ]);
  viewport = new MeshViewport(el("viewport"), atlas, {
    gridSize: 16,
    effects: createEffectRegistry(bookTexture),
    label: "独立子区块三维预览，可拖动旋转和滚轮缩放",
  });
  const gizmo = new AxisGizmo(el<HTMLCanvasElement>("gizmo"));
  viewport.observeCamera((q) => gizmo.update(q));
  el("version").textContent = `JAVA ${catalog.version}`;
  for (const scene of catalog.scenes) {
    const button = document.createElement("button");
    button.className = "scene-card";
    button.dataset.scene = scene.id;
    const copy = document.createElement("span");
    copy.className = "copy";
    const title = document.createElement("strong");
    title.textContent = scene.title;
    const description = document.createElement("small");
    description.textContent = scene.description;
    copy.append(title, description);
    button.append(copy);
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
  for (const id of ["fix", "cull", "greedy"])
    el<HTMLInputElement>(id).onchange = () => void load();
  // View chips toggle their own class and drive the viewport.
  const toggleChip = (id: string, apply: (on: boolean) => void): void => {
    const chip = el<HTMLButtonElement>(id);
    chip.onclick = () => {
      const on = !chip.classList.contains("on");
      chip.classList.toggle("on", on);
      chip.setAttribute("aria-pressed", String(on));
      apply(on);
    };
  };
  toggleChip("wireframe", (on) => {
    wireframe = on;
    viewport.showWireframe(on);
  });
  toggleChip("frame", (on) => viewport.showFrame(on));
  toggleChip("grid", (on) => viewport.showGrid(on));
  el("reset").onclick = () => viewport.reset();
  // 面板收到侧面:收起时整页只剩场景与右上角视图选项,留一个把手拉回来。
  const panel = el("panel");
  const opener = el("panel-open");
  const setPanel = (collapsed: boolean): void => {
    panel.classList.toggle("collapsed", collapsed);
    opener.hidden = !collapsed;
  };
  el("panel-collapse").onclick = () => setPanel(true);
  opener.onclick = () => setPanel(false);
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
    if (latest) download(latest, `${current.id}.nvframe`);
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
  const first = catalog.scenes[0];
  if (!first) {
    status("目录里没有可用场景", true);
    el("scene-title").textContent = "无场景";
    return;
  }
  current = first;
  document.querySelector(".scene-card")?.classList.add("selected");
  await load(true);
  window.addEventListener(
    "pagehide",
    () => {
      viewport.dispose();
      gizmo.dispose();
      atlas.dispose();
      bookTexture.dispose();
    },
    { once: true },
  );
}
void start().catch(fail);
