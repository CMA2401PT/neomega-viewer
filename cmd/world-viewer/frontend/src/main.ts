import * as THREE from "three";
import {
  loadAtlas,
  createEffectRegistry,
  enchantingTableBookTextureURL,
  SectionScene,
  type EffectRegistry,
  type SectionObject,
} from "@neomega/viewer/render";
import { ChunkStream, type SectionKey, type StreamStats } from "./stream.ts";
import { decodeColumn } from "./packet.ts";
import { createWorldPicker, type WorldInfo } from "./picker.ts";
import { FrameRate, geometryMemoryBytes } from "./metrics.ts";
import { createDiagnostics } from "./diagnostics.ts";
import { Backdrop, lineArtStyle } from "./backdrop.ts";
import { createPanelToggle } from "./panel.ts";
import { createStage } from "./stage.ts";
import "./style.css";

const element = <T extends HTMLElement>(id: string): T =>
  document.getElementById(id) as T;
const host = element<HTMLDivElement>("scene");
const status = element<HTMLOutputElement>("status");
const coordinateDisplay = element<HTMLOutputElement>("coordinates");
const fpsDisplay = element<HTMLOutputElement>("fps");
const memoryDisplay = element<HTMLOutputElement>("memory");
const frameRate = new FrameRate();
const stage = createStage();
const diagnostics = createDiagnostics(
  element<HTMLOutputElement>("issue-count"),
  element<HTMLUListElement>("issue-list"),
  element<HTMLParagraphElement>("issue-empty"),
);
function resetFrameRate(): void {
  frameRate.reset();
  fpsDisplay.value = "FPS —";
}
document.addEventListener("visibilitychange", resetFrameRate);
const fields = ["x", "y", "z"].map((id) => element<HTMLInputElement>(id));
const radiusInput = element<HTMLInputElement>("radius");
const wireframe = element<HTMLInputElement>("wireframe");
const occlusion = element<HTMLInputElement>("occlusion");
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
// Section placement is fixed. Update it on attachment, and animated frames explicitly.
scene.matrixWorldAutoUpdate = false;
let worldScene: SectionScene | undefined;
const camera = new THREE.PerspectiveCamera(65, 1, 0.1, 180);
camera.position.set(0, 80, 0);
const rotation = new THREE.Euler(-0.35, 0, 0, "YXZ");
camera.quaternion.setFromEuler(rotation);
const axes = element<HTMLCanvasElement>("axes");
const axisContext = axes.getContext("2d")!;
axes.width = axes.height = 80 * Math.min(devicePixelRatio, 2);
const forward = new THREE.Vector3(),
  right = new THREE.Vector3();
const pressed = new Set<string>();
let atlas: THREE.Texture | undefined, book: THREE.Texture | undefined;
let effects: EffectRegistry = new Map();
type ColumnSection = ReturnType<typeof decodeColumn>["sections"][number];
// 加载统计属于列协议；共享场景只持有绘制资源。
interface LoadedSection {
  section: SectionObject;
  unknown: number;
  bytes: number;
  dispose(): void;
}
interface ColumnPayload {
  buffer: ArrayBuffer;
  column: [number, number];
}
let stream: ChunkStream<LoadedSection, ColumnPayload, Map<number, ColumnSection>> | undefined;
// 默认场景以环为中心加载，这个半径刚好盖住整片海。
const backdropRadius = 18;
const worldFov = 65;
let streaming = false,
  dragging = false,
  disposed = false;
let lastTime = performance.now(),
  lastHUD = 0;
let failure = "";
let currentWorld: WorldInfo | undefined;
// 进入默认场景前的视距，打开存档时还给用户。
let radiusBeforeBackdrop: string | undefined;
let backdrop: Backdrop | undefined;
// 相机静止时轻轻漂移，线框随之闪动成波光；用户一动手就停。
let swayOrigin: THREE.Vector3 | undefined;
// 默认场景的点亮时机：加载完自动开始；鼠标一动就提前，不必等远处的空区块。
let revealWanted = false;
let streamStats: StreamStats | undefined;
let worldEpoch = 0;
let textures: Promise<void> | undefined;
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
  const unknown = [...(stream?.loaded.values() ?? [])].reduce(
    (total, column) => total + column.unknown,
    0,
  );
  status.textContent = `${stats.loaded} / ${stats.wanted} 子区块${stats.pending ? " · 载入中" : ""}${unknown ? ` · ${unknown} 个方块缺少模型（紫色）` : ""}`;
}
async function fetchColumn(
  keys: SectionKey[],
  signal: AbortSignal,
  revision: number,
): Promise<ColumnPayload> {
  const first = keys[0],
    last = keys[keys.length - 1];
  const query = new URLSearchParams({
    world: String(revision),
    x: String(first.x),
    z: String(first.z),
    minY: String(first.y),
    maxY: String(last.y),
  });
  const response = await fetch(`/api/chunk?${query}`, {
    signal,
    cache: "no-store",
  });
  if (!response.ok)
    throw new Error(
      `区块 ${first.x}, ${first.z}：${(await response.text()).trim()}`,
    );
  const buffer = await response.arrayBuffer();
  signal.throwIfAborted();
  return { buffer, column: [first.x, first.z] };
}

function clampPosition(): void {
  camera.position.x = THREE.MathUtils.clamp(
    camera.position.x,
    -29_999_999,
    29_999_999,
  );
  camera.position.z = THREE.MathUtils.clamp(
    camera.position.z,
    -29_999_999,
    29_999_999,
  );
  camera.position.y = THREE.MathUtils.clamp(camera.position.y, -2048, 2047);
}
function refresh(): void {
  clampPosition();
  const radius = Number(radiusInput.value);
  camera.far = backdrop ? 2400 : (radius + 2) * 16 * 1.7;
  camera.updateProjectionMatrix();
  scene.fog = backdrop
    ? null
    : new THREE.Fog(worldFogColor, Math.max(16, (radius - 0.6) * 16), camera.far);
  // 默认场景是有限的一片，加载中心钉在环上，走到哪里都看得到全貌。
  const anchor = backdrop ? new THREE.Vector3(...backdrop.info.center) : camera.position;
  if (streaming)
    stream?.update(
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
  swayOrigin = undefined;
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
  if (backdrop) backdrop.setLineArt(wireframe.checked);
  else worldScene?.setWireframe(wireframe.checked);
});
occlusion.addEventListener("change", () => worldScene?.setOcclusion(occlusion.checked));
radiusInput.addEventListener("input", () => {
  element<HTMLOutputElement>("radius-label").value = radiusInput.value;
  refresh();
});
renderer.domElement.addEventListener("pointerdown", (event) => {
  if (event.button !== 0 && event.button !== 2) return;
  renderer.domElement.focus();
  stopSway();
  dragging = true;
  renderer.domElement.setPointerCapture(event.pointerId);
});
renderer.domElement.addEventListener("pointermove", (event) => {
  if (!dragging) return;
  rotation.y -= event.movementX * 0.004;
  rotation.x = THREE.MathUtils.clamp(
    rotation.x - event.movementY * 0.004,
    -Math.PI / 2 + 0.01,
    Math.PI / 2 - 0.01,
  );
  camera.quaternion.setFromEuler(rotation);
});
renderer.domElement.addEventListener("pointerup", () => {
  dragging = false;
});
renderer.domElement.addEventListener("pointercancel", () => {
  dragging = false;
});
renderer.domElement.addEventListener("contextmenu", (event) =>
  event.preventDefault(),
);
renderer.domElement.addEventListener(
  "wheel",
  (event) => {
    event.preventDefault();
    stopSway();
    camera.getWorldDirection(forward);
    camera.position.addScaledVector(
      forward,
      -Math.sign(event.deltaY) * (event.shiftKey ? 32 : 8),
    );
    refresh();
  },
  { passive: false },
);
renderer.domElement.addEventListener("keydown", (event) => {
  if (
    [
      "KeyW",
      "KeyA",
      "KeyS",
      "KeyD",
      "Space",
      "ShiftLeft",
      "ShiftRight",
    ].includes(event.code)
  ) {
    event.preventDefault();
    stopSway();
    pressed.add(event.code);
  }
});
window.addEventListener("keyup", (event) => pressed.delete(event.code));
renderer.domElement.addEventListener("blur", () => pressed.clear());
window.addEventListener("blur", () => {
  pressed.clear();
  dragging = false;
});
const resize = new ResizeObserver(() => {
  updateResolution();
  renderer.setSize(host.clientWidth, host.clientHeight, false);
  camera.aspect = host.clientWidth / host.clientHeight;
  camera.updateProjectionMatrix();
  backdrop?.resize(renderer);
});
resize.observe(host);
function drawAxes(): void {
  const scale = axes.width / 80;
  axisContext.setTransform(scale, 0, 0, scale, 0, 0);
  axisContext.clearRect(0, 0, 80, 80);
  const inverse = camera.quaternion.clone().invert();
  const directions = [
    new THREE.Vector3(1, 0, 0),
    new THREE.Vector3(0, 1, 0),
    new THREE.Vector3(0, 0, 1),
  ]
    .map((v, i) => ({
      v: v.applyQuaternion(inverse),
      label: "XYZ"[i],
      color: ["#b63c3c", "#36733c", "#315b9a"][i],
    }))
    .sort((a, b) => a.v.z - b.v.z);
  axisContext.font = "12px ui-monospace,monospace";
  axisContext.textAlign = "center";
  axisContext.textBaseline = "middle";
  for (const { v, label, color } of directions) {
    axisContext.strokeStyle = axisContext.fillStyle = color;
    axisContext.lineWidth = 2;
    axisContext.beginPath();
    axisContext.moveTo(38, 40);
    axisContext.lineTo(38 + v.x * 24, 40 - v.y * 24);
    axisContext.stroke();
    axisContext.fillText(label, 38 + v.x * 33, 40 - v.y * 33);
  }
}
function drawScene(): void {
  if (worldScene) worldScene.render(renderer, camera);
  else renderer.render(scene, camera);
}
/** 加载按距离由近到远，四分之一时环和近处的海已经齐了，可以点亮。 */
function tryReveal(): void {
  if (!backdrop || !streamStats?.loaded) return;
  const ready = !streamStats.pending
    || (revealWanted && streamStats.loaded >= streamStats.wanted / 4);
  if (ready) backdrop.beginReveal(performance.now());
}
for (const type of ["pointermove", "pointerdown"] as const)
  window.addEventListener(type, () => {
    if (revealWanted) return;
    revealWanted = true;
    tryReveal();
  });
function stopSway(): void {
  if (swayOrigin) camera.position.copy(swayOrigin);
  swayOrigin = undefined;
}
function animate(now: number): void {
  if (disposed) return;
  const dt = Math.min(0.05, (now - lastTime) / 1000);
  lastTime = now;
  if (pressed.size) {
    const speed = 24 * dt;
    // 用偏航角构造水平基向量，抬头或低头不会改变 WASD 的高度和速度。
    forward.set(-Math.sin(rotation.y), 0, -Math.cos(rotation.y));
    right.set(Math.cos(rotation.y), 0, -Math.sin(rotation.y));
    camera.position.addScaledVector(
      forward,
      (Number(pressed.has("KeyW")) - Number(pressed.has("KeyS"))) * speed,
    );
    camera.position.addScaledVector(
      right,
      (Number(pressed.has("KeyD")) - Number(pressed.has("KeyA"))) * speed,
    );
    camera.position.y +=
      (Number(pressed.has("Space")) -
        Number(pressed.has("ShiftLeft") || pressed.has("ShiftRight"))) * speed;
    camera.position.x = THREE.MathUtils.clamp(
      camera.position.x,
      -29_999_999,
      29_999_999,
    );
    camera.position.z = THREE.MathUtils.clamp(
      camera.position.z,
      -29_999_999,
      29_999_999,
    );
    camera.position.y = THREE.MathUtils.clamp(camera.position.y, -2048, 2047);
    refresh();
  }
  if (swayOrigin) {
    const t = now / 1000;
    camera.position.set(
      swayOrigin.x + Math.sin(t * 0.11) * 3.2,
      swayOrigin.y + Math.sin(t * 0.17) * 0.5,
      swayOrigin.z + Math.cos(t * 0.07) * 1.5,
    );
  }
  stream?.processFrame();
  worldScene?.update(camera, now / 1000);
  camera.updateMatrixWorld();
  if (backdrop) backdrop.render(camera, now);
  else drawScene();
  stage.update(camera, now);
  const fps = frameRate.sample(now);
  if (fps !== undefined) {
    fpsDisplay.value = `FPS ${fps.toFixed(0)}`;
    memoryDisplay.value = `网格内存 ≈ ${(geometryMemoryBytes(scene) / 1024 ** 2).toFixed(1)} MiB`;
  }
  if (now - lastHUD > 80) {
    drawAxes();
    coordinateDisplay.value = `X ${camera.position.x.toFixed(1)}　Y ${camera.position.y.toFixed(1)}　Z ${camera.position.z.toFixed(1)}`;
    lastHUD = now;
  }
  requestAnimationFrame(animate);
}
requestAnimationFrame(animate);
async function loadTextures(): Promise<void> {
  atlas = await loadAtlas("/resources/atlas.png");
  try {
    book = await loadAtlas(enchantingTableBookTextureURL);
    effects = createEffectRegistry(book);
  } catch {
    /* The table's static shape remains available. */
  }
}
function lookAt(target: [number, number, number]): void {
  camera.lookAt(...target);
  rotation.setFromQuaternion(camera.quaternion);
}
/** 默认场景走线描画面；存档恢复实体绘制与明亮背景，用户调过的视距也一并还原。 */
function applyAtmosphere(info: WorldInfo): void {
  document.body.dataset.scene = info.backdrop ? "backdrop" : "world";
  wireframe.checked = info.backdrop !== undefined;
  backdrop?.dispose();
  backdrop = undefined;
  if (info.backdrop) {
    camera.position.set(...info.position);
    camera.fov = info.backdrop.fov;
    camera.updateProjectionMatrix();
    backdrop = new Backdrop(renderer, camera, info.backdrop, drawScene);
    scene.background = null;
    radiusBeforeBackdrop ??= radiusInput.value;
    radiusInput.value = String(backdropRadius);
  } else {
    camera.fov = worldFov;
    scene.background = new THREE.Color(worldFogColor);
    if (radiusBeforeBackdrop !== undefined) radiusInput.value = radiusBeforeBackdrop;
    radiusBeforeBackdrop = undefined;
  }
  stage.show(info.backdrop);
  element<HTMLOutputElement>("radius-label").value = radiusInput.value;
}
async function showWorld(info: WorldInfo): Promise<void> {
  if (currentWorld && info.revision < currentWorld.revision) return;
  const epoch = ++worldEpoch;
  diagnostics.clear();
  stream?.dispose();
  stream = undefined;
  worldScene?.dispose();
  worldScene = undefined;
  memoryDisplay.value = "网格内存 ≈ 0.0 MiB";
  pressed.clear();
  dragging = false;
  failure = "";
  currentWorld = info;
  streamStats = undefined;
  streaming = info.loaded || info.backdrop !== undefined;
  applyAtmosphere(info);
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
  camera.position.set(...info.position);
  swayOrigin = info.backdrop ? camera.position.clone() : undefined;
  if (info.backdrop) lookAt(info.backdrop.look);
  else {
    // 存档沿用一贯的初始朝向：朝北略微俯视，不继承默认场景的取景。
    rotation.set(-0.35, 0, 0, "YXZ");
    camera.quaternion.setFromEuler(rotation);
  }
  syncFields();
  refresh();
  updateStatus();
  if (!streaming) return;
  if (info.loaded) status.textContent = "载入材质…";
  textures ??= loadTextures().catch((error) => {
    textures = undefined;
    throw error;
  });
  await textures;
  if (disposed || epoch !== worldEpoch) return;
  const draw = new SectionScene(
    scene,
    atlas!,
    effects,
    renderer,
    16,
    info.backdrop ? lineArtStyle : undefined,
  );
  draw.setWireframe(wireframe.checked && !info.backdrop);
  draw.setOcclusion(occlusion.checked && !info.backdrop);
  worldScene = draw;
  stream = new ChunkStream({
    load: (keys, signal) => fetchColumn(keys, signal, info.revision),
    decode: (payload: ColumnPayload) => {
      const data = decodeColumn(payload.buffer);
      diagnostics.add(data.issues, payload.column);
      return new Map(data.sections.map((section) => [section.y, section]));
    },
    prepare: (key, sections) => {
      const section = sections.get(key.y);
      const object = draw.prepare(key, section?.frame);
      return {
        section: object,
        unknown: section?.unknown ?? 0,
        bytes: object.bytes,
        dispose: () => object.dispose(),
      };
    },
    attach: (_key, value) => draw.add(value.section),
    changed: (stats) => {
      updateStatus(stats);
      streamStats = stats;
      tryReveal();
    },
  });
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
  stream?.dispose();
  worldScene?.dispose();
  resize.disconnect();
  window.removeEventListener("resize", updateResolution);
  document.removeEventListener("visibilitychange", resetFrameRate);
  renderer.dispose();
  backdrop?.dispose();
  atlas?.dispose();
  book?.dispose();
});
