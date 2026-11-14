import * as THREE from "three";
import type { StreamStats } from "../scene/stream.ts";
import { MeshWorld } from "../scene/mesh-world.ts";
import { SceneAssets } from "../scene/scene-assets.ts";
import { FlightControls } from "../scene/flight.ts";
import { HUD } from "../ui/hud.ts";
import { createWorldPicker } from "../ui/picker.ts";
import type { WorldInfo } from "../api/world-info.ts";
import { createDiagnostics } from "../ui/diagnostics.ts";
import { fileAppearance } from "../ui/file-kind.ts";
import { mountDock } from "../ui/dock.ts";
import { bindMovePad } from "../ui/movepad.ts";
import { anchorPopovers, closePopovers } from "../ui/popover.ts";
import { showToast } from "../ui/toast.ts";
import { lineArtStyle } from "../welcome/backdrop.ts";
import { Intro } from "../welcome/intro.ts";
import { BrickmapWorld } from "../terrain/brickmap-world.ts";
import { FarView, FAR_FOG } from "../terrain/far-view.ts";
import { WINDOW_SIZE } from "../terrain/map-regions.ts";
import { Minimap } from "../minimap/minimap.ts";

const SOURCE_KINDS: Record<string, string> = {
  directory: "Java 存档",
  pack: "Java PackFS 存档",
  bedrock: "基岩存档",
};
// Java 维度是存档内相对目录，基岩维度是编号；其他自定义维度原样显示。
const DIMENSIONS: Record<string, string> = {
  ".": "主世界",
  "DIM-1": "下界",
  "DIM1": "末地",
  "0": "主世界",
  "1": "下界",
  "2": "末地",
};

export function startViewer(): void {
  // 这里组装各视图，并保持“上传、远景、精细网格、小地图”的帧内顺序。
  const element = <T extends HTMLElement>(id: string): T =>
    document.getElementById(id) as T;
  const host = element<HTMLDivElement>("scene");
  const loading = element<HTMLOutputElement>("loading");
  const diagnostics = createDiagnostics(
    element<HTMLOutputElement>("issue-count"),
    element<HTMLUListElement>("issue-list"),
    element<HTMLParagraphElement>("issue-empty"),
  );
  const radiusInput = element<HTMLInputElement>("radius");
  const wireframe = element<HTMLInputElement>("wireframe");
  const farEnabled = element<HTMLInputElement>("far-view");
  const farStatus = element<HTMLParagraphElement>("far-status");
  const resolution = element<HTMLSelectElement>("resolution");
  const renderer = new THREE.WebGLRenderer({
    antialias: true,
    powerPreference: "high-performance",
  });
  function updateResolution(): void {
    const ratio = resolution.value === "auto"
      ? Math.min(devicePixelRatio, 1.5)
      : resolution.value === "native" ? devicePixelRatio : Number(resolution.value);
    renderer.setPixelRatio(ratio);
  }
  updateResolution();
  resolution.addEventListener("change", updateResolution);
  window.addEventListener("resize", updateResolution);
  renderer.outputColorSpace = THREE.SRGBColorSpace;
  renderer.domElement.tabIndex = 0;
  renderer.domElement.setAttribute(
    "aria-label",
    "三维场景，拖动转向，WASD 水平移动，Shift 下降，空格上升",
  );
  host.append(renderer.domElement);
  const worldFogColor = 0xd9dee3;
  const scene = new THREE.Scene();
  scene.background = new THREE.Color(worldFogColor);
  // 静态 section 只在挂载时更新变换，动画对象由每帧更新。
  scene.matrixWorldAutoUpdate = false;
  let meshes: MeshWorld | undefined;
  const assets = new SceneAssets();
  const camera = new THREE.PerspectiveCamera(65, 1, 0.1, 180);
  camera.position.set(0, 80, 0);
  const controls = new FlightControls(camera, renderer.domElement, {
    paused: () => minimap.expanded,
    interaction: () => intro.stopSway(),
    changed: refresh,
  });
  const intro = new Intro({
    renderer,
    camera,
    scene,
    controls,
    wireframe,
    radius: radiusInput,
    background: worldFogColor,
    draw: drawScene,
  });
  const terrain = new BrickmapWorld(renderer.getContext() as WebGL2RenderingContext);
  const farView = new FarView(renderer, terrain);
  let distantEnabled = false;
  const minimap = new Minimap(renderer, terrain, jump, () => {
    controls.cancel();
    intro.stopSway();
  }, () => renderer.domElement.focus());
  const hud = new HUD(jump);
  let disposed = false;
  let lastTime = performance.now(),
    lastStatus = 0;
  let currentWorld: WorldInfo | undefined;
  let worldEpoch = 0;
  let lastError = "";

  function showStats(stats: StreamStats): void {
    // 顶部读数只在还有区块排队时出现；累计数字和缺模型数量留在场景信息里。
    loading.hidden = !stats.pending;
    loading.value = `载入中 ${stats.loaded} / ${stats.wanted}`;
    const unknown = meshes?.unknown ?? 0;
    element("info-loaded").textContent =
      `${stats.loaded} / ${stats.wanted} 个子区块${unknown ? ` · ${unknown} 个方块缺少模型（紫色）` : ""}`;
    if (stats.error && stats.error !== lastError) showToast(stats.error);
    lastError = stats.error;
  }
  function showWorldInfo(info: WorldInfo): void {
    const name = info.backdrop ? "Glass Sea" : info.loaded ? info.name : "未打开存档";
    element("scene-name").textContent = name;
    element("info-name").textContent = name;
    element("info-kind").textContent = info.backdrop
      ? "打开文件或存档后替换"
      : [
        SOURCE_KINDS[info.kind ?? ""] ?? (info.kind ? fileAppearance(info.kind, false).label : ""),
        info.dimension ? DIMENSIONS[info.dimension] ?? info.dimension : "",
      ].filter(Boolean).join(" · ");
    const path = element("info-path");
    path.textContent = info.path ?? "";
    path.hidden = !info.path;
    const conversion = element("info-conversion");
    conversion.hidden = info.conversionSeconds === undefined && !info.reusedStructure;
    conversion.lastElementChild!.textContent = info.conversionSeconds !== undefined
      ? `用时 ${info.conversionSeconds.toFixed(1)} 秒`
      : "已复用同名存档，无需转换";
    element("info-loaded").textContent = "—";
    element<HTMLButtonElement>("open-world").hidden = info.canOpen === false;
    element<HTMLButtonElement>("map-open").disabled = !info.minimap;
  }
  function refresh(): void {
    controls.clamp();
    const radius = Number(radiusInput.value);
    distantEnabled = Boolean(currentWorld?.loaded && currentWorld.minimap && farEnabled.checked);
    camera.far = intro.backdrop ? 2400 : distantEnabled ? WINDOW_SIZE : (radius + 2) * 16 * 1.7;
    camera.updateProjectionMatrix();
    scene.fog = intro.backdrop
      ? null
      : distantEnabled ? new THREE.Fog(worldFogColor, ...FAR_FOG)
        : new THREE.Fog(worldFogColor, Math.max(16, (radius - 0.6) * 16), camera.far);
    // 默认场景是有限的一片，加载中心钉在环上，走到哪里都看得到全貌。
    const anchor = intro.backdrop ? new THREE.Vector3(...intro.backdrop.info.center) : camera.position;
    meshes?.stream.update(
      Math.floor(anchor.x / 16),
      Math.floor(anchor.z / 16),
      Math.max(-128, Math.min(127, Math.floor(anchor.y / 16))),
      radius,
    );
  }
  function jump(position: [number, number, number]): void {
    if (
      position.some((n) => !Number.isFinite(n)) ||
      Math.abs(position[0]) >= 30_000_000 ||
      Math.abs(position[2]) >= 30_000_000 ||
      position[1] < -2048 ||
      position[1] >= 2048
    ) {
      showToast("坐标超出范围");
      return;
    }
    intro.stopSway();
    camera.position.set(...position);
    refresh();
    renderer.domElement.focus();
  }
  wireframe.addEventListener("change", () => {
    if (intro.backdrop) intro.backdrop.setLineArt(wireframe.checked);
    else meshes?.scene.setWireframe(wireframe.checked);
  });
  farEnabled.addEventListener("change", refresh);
  radiusInput.addEventListener("input", () => {
    element<HTMLOutputElement>("radius-label").value = radiusInput.value;
    refresh();
  });
  const resize = new ResizeObserver(() => {
    updateResolution();
    renderer.setSize(host.clientWidth, host.clientHeight, false);
    camera.aspect = host.clientWidth / host.clientHeight;
    camera.updateProjectionMatrix();
    intro.backdrop?.resize(renderer);
  });
  resize.observe(host);
  function drawScene(): void {
    renderer.render(scene, camera);
  }
  function animate(now: number): void {
    if (disposed) return;
    const dt = Math.min(0.05, (now - lastTime) / 1000);
    lastTime = now;
    controls.update(dt);
    intro.moveCamera(now);
    meshes?.stream.processFrame();
    meshes?.scene.update(camera, now / 1000);
    camera.updateMatrixWorld();
    minimap.update(camera.position.x, camera.position.y, camera.position.z, controls.rotation.y);
    terrain.far.enabled = distantEnabled;
    terrain.far.center = [camera.position.x, camera.position.z];
    terrain.update();
    renderer.resetState();
    if (intro.backdrop) intro.backdrop.render(camera, now);
    else if (distantEnabled && terrain.far.gpu) {
      renderer.setRenderTarget(null);
      renderer.setScissorTest(false);
      renderer.setClearColor(worldFogColor);
      renderer.clear();
      farView.draw(camera, meshes?.revision ?? 0, () => meshes?.sections() ?? []);
      // scene.background 为颜色时 three 会强制清屏，远景之后显式保留颜色和深度。
      const background = scene.background;
      const autoClear = renderer.autoClear;
      scene.background = null;
      renderer.autoClear = false;
      try { drawScene(); }
      finally { scene.background = background; renderer.autoClear = autoClear; }
    } else {
      farView.dispose();
      drawScene();
    }
    hud.update(now, camera, scene);
    minimap.render();
    if (now - lastStatus > 250) {
      farStatus.hidden = !distantEnabled;
      if (distantEnabled) {
        const pending = terrain.pending(terrain.far);
        farStatus.textContent = terrain.full(terrain.far) ? "远景显存已满，部分区域暂缺"
          : terrain.error ? `远景：${terrain.error}`
            : pending ? `远景 · ${pending} 个区域待载入` : "远景已就绪";
      }
      lastStatus = now;
    }
    requestAnimationFrame(animate);
  }
  requestAnimationFrame(animate);
  async function showWorld(info: WorldInfo): Promise<void> {
    if (currentWorld && info.revision < currentWorld.revision) return;
    const epoch = ++worldEpoch;
    diagnostics.clear();
    meshes?.dispose();
    meshes = undefined;
    hud.clearMemory();
    controls.cancel();
    lastError = "";
    currentWorld = info;
    terrain.setWorld(info);
    minimap.setWorld(info);
    intro.show(info);
    element<HTMLOutputElement>("radius-label").value = radiusInput.value;
    showWorldInfo(info);
    refresh();
    if (!info.loaded && !info.backdrop) return;
    loading.value = "载入材质";
    loading.hidden = false;
    await assets.load();
    if (disposed || epoch !== worldEpoch) return;
    meshes = new MeshWorld({
      scene,
      atlas: assets.atlas!,
      effects: assets.effects,
      style: info.backdrop ? lineArtStyle : undefined,
      revision: info.revision,
      issues: diagnostics.add,
      changed: (stats) => {
        showStats(stats);
        intro.loading(stats);
      },
    });
    meshes.scene.setWireframe(wireframe.checked && !info.backdrop);
    refresh();
  }

  // 界面外壳：两个梯形底座、弹出面板、移动按钮、隐藏开关。
  const hudElement = element("hud");
  mountDock(hudElement);
  mountDock(element("map-dock"));
  anchorPopovers(hudElement);
  bindMovePad(element("movepad"), controls);
  const eye = element<HTMLButtonElement>("eye");
  eye.addEventListener("click", () => {
    const hidden = document.body.toggleAttribute("data-chrome-hidden");
    const label = hidden ? "显示界面" : "隐藏界面";
    eye.setAttribute("aria-pressed", String(hidden));
    eye.setAttribute("aria-label", label);
    eye.title = label;
    minimap.concealed = hidden;
    // 隐藏时手指可能还按在移动按钮上，按钮不可见后收不到松开事件。
    controls.cancel();
    if (hidden) closePopovers();
  });
  element("map-open").addEventListener("click", () => {
    closePopovers();
    minimap.setExpanded(true);
  });
  const openPicker = createWorldPicker(
    (info) => {
      void showWorld(info).catch(showError);
    },
    () => currentWorld,
  );
  element("open-world").addEventListener("click", () => {
    closePopovers();
    openPicker();
  });
  function showError(error: unknown): void {
    loading.hidden = true;
    showToast(error instanceof Error ? error.message : String(error));
  }
  async function start(): Promise<void> {
    const response = await fetch("/api/world", { cache: "no-store" });
    if (!response.ok) throw new Error("无法读取存档信息");
    const info: WorldInfo = await response.json();
    element<HTMLOutputElement>("version").value = info.version;
    await showWorld(info);
  }
  void start().catch(showError);
  window.addEventListener("pagehide", () => {
    disposed = true;
    minimap.dispose();
    farView.dispose();
    terrain.dispose();
    meshes?.dispose();
    resize.disconnect();
    window.removeEventListener("resize", updateResolution);
    controls.dispose();
    hud.dispose();
    renderer.dispose();
    intro.dispose();
    assets.dispose();
  });
}
