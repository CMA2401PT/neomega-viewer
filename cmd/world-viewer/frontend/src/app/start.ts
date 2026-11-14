import * as THREE from "three";
import type { StreamStats } from "../scene/stream.ts";
import { MeshWorld } from "../scene/mesh-world.ts";
import { SceneAssets } from "../scene/scene-assets.ts";
import { FlightControls } from "../scene/flight.ts";
import { HUD } from "../ui/hud.ts";
import { createWorldPicker } from "../ui/picker.ts";
import type { WorldInfo } from "../api/world-info.ts";
import { createDiagnostics } from "../ui/diagnostics.ts";
import { lineArtStyle } from "../welcome/backdrop.ts";
import { createPanelToggle } from "../ui/panel.ts";
import { Intro } from "../welcome/intro.ts";
import { BrickmapWorld } from "../terrain/brickmap-world.ts";
import { FarView, FAR_FOG } from "../terrain/far-view.ts";
import { WINDOW_SIZE } from "../terrain/map-regions.ts";
import { Minimap } from "../minimap/minimap.ts";


export function startViewer(): void {
  // 这里组装各视图，并保持“上传、远景、精细网格、小地图”的帧内顺序。
  const element = <T extends HTMLElement>(id: string): T =>
    document.getElementById(id) as T;
  const host = element<HTMLDivElement>("scene");
  const status = element<HTMLOutputElement>("status");
  const hud = new HUD();
  const diagnostics = createDiagnostics(
    element<HTMLOutputElement>("issue-count"),
    element<HTMLUListElement>("issue-list"),
    element<HTMLParagraphElement>("issue-empty"),
  );
  const fields = ["x", "y", "z"].map((id) => element<HTMLInputElement>(id));
  const radiusInput = element<HTMLInputElement>("radius");
  const wireframe = element<HTMLInputElement>("wireframe");
  const farEnabled = element<HTMLInputElement>("far-view");
  const farStatus = element<HTMLOutputElement>("far-status");
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
  let streaming = false,
    disposed = false;
  let lastTime = performance.now(),
    lastHUD = 0;
  let failure = "";
  let currentWorld: WorldInfo | undefined;
  let worldEpoch = 0;
  function updateStatus(stats?: StreamStats): void {
    const error = failure || stats?.error;
    status.dataset.error = String(Boolean(error));
    if (error) {
      status.textContent = error;
      return;
    }
    if (!currentWorld?.loaded) {
      status.textContent = "未打开存档";
      return;
    }
    if (!stats) return;
    const unknown = meshes?.unknown ?? 0;
    status.textContent = `${stats.loaded} / ${stats.wanted} 子区块${stats.pending ? " · 载入中" : ""}${unknown ? ` · ${unknown} 个方块缺少模型（紫色）` : ""}`;
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
    if (streaming)
      meshes?.stream.update(
        Math.floor(anchor.x / 16),
        Math.floor(anchor.z / 16),
        Math.max(-128, Math.min(127, Math.floor(anchor.y / 16))),
        radius,
      );
  }
  function syncFields(): void {
    camera.position.toArray().forEach((value, i) => {
      fields[i].value = value.toFixed(1);
    });
  }
  function jump(position: [number, number, number]): void {
    if (
      position.some((n) => !Number.isFinite(n)) ||
      Math.abs(position[0]) >= 30_000_000 ||
      Math.abs(position[2]) >= 30_000_000 ||
      position[1] < -2048 ||
      position[1] >= 2048
    ) {
      failure = "坐标超出范围";
      updateStatus();
      return;
    }
    failure = "";
    intro.stopSway();
    camera.position.set(...position);
    syncFields();
    refresh();
    renderer.domElement.focus();
  }
  element<HTMLFormElement>("teleport").addEventListener("submit", (event) => {
    event.preventDefault();
    jump(fields.map((field) => Number(field.value)) as [number, number, number]);
  });
  wireframe.addEventListener("change", () => {
    wireframe.dataset.tried = "";
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
    intro.stage.update(camera, now);
    hud.update(now, camera, scene);
    minimap.render();
    if (now - lastHUD > 80) {
      farStatus.hidden = !distantEnabled;
      if (distantEnabled) {
        const pending = terrain.pending(terrain.far);
        farStatus.value = terrain.full(terrain.far) ? "远景显存已满，部分区域暂缺"
          : terrain.error ? `远景：${terrain.error}`
            : pending ? `远景 · ${pending} 区域待载入` : "远景 · brickmap";
      }
      lastHUD = now;
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
    failure = "";
    currentWorld = info;
    terrain.setWorld(info);
    minimap.setWorld(info);
    streaming = info.loaded || info.backdrop !== undefined;
    intro.show(info);
    element<HTMLOutputElement>("radius-label").value = radiusInput.value;
    element<HTMLOutputElement>("world-name").value = info.loaded
      ? info.name
      : "未打开存档";
    element<HTMLOutputElement>("world-name").title = info.path ?? "";
    const conversion = element<HTMLOutputElement>("conversion-summary");
    conversion.hidden = info.conversionSeconds === undefined && !info.reusedStructure;
    conversion.value = info.conversionSeconds !== undefined
      ? `转换完成 · 用时 ${info.conversionSeconds.toFixed(1)} 秒`
      : info.reusedStructure ? "已复用同名存档，无需转换" : "";
    element<HTMLElement>("open-world-label").textContent = info.loaded
      ? "切换存档"
      : "打开存档";
    element<HTMLButtonElement>("open-world").hidden = info.canOpen === false;
    syncFields();
    refresh();
    updateStatus();
    if (!streaming) return;
    if (info.loaded) status.textContent = "载入材质…";
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
        updateStatus(stats);
        intro.loading(stats);
      },
    });
    meshes.scene.setWireframe(wireframe.checked && !info.backdrop);
    refresh();
  }
  createPanelToggle();
  createWorldPicker(
    (info) => {
      void showWorld(info).catch(showError);
    },
    () => currentWorld,
  );
  function showError(error: unknown): void {
    failure = error instanceof Error ? error.message : String(error);
    updateStatus();
  }
  async function start(): Promise<void> {
    const response = await fetch("/api/world", { cache: "no-store" });
    if (!response.ok) throw new Error("无法读取存档信息");
    const info: WorldInfo = await response.json();
    element<HTMLOutputElement>("version").value = info.version;
    await showWorld(info);
  }
  void start().catch((error: unknown) => {
    failure = error instanceof Error ? error.message : String(error);
    updateStatus();
  });
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
