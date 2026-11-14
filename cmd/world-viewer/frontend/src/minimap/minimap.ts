import type { WorldInfo } from "../api/world-info.ts";
import { cameraRay, type Ray, type PickResult } from "../../../../../../../neomega-viewer-core/viewer/brickmap/web/renderer.ts";
import { cameraBasis, type Camera, type Vec3 } from "../../../../../../../neomega-viewer-core/viewer/brickmap/web/volume.ts";
import { REGION_SIZE, WINDOW_SIZE, regionKey, windowOrigin, type RegionPosition } from "../terrain/map-regions.ts";
import type { BrickmapWorld } from "../terrain/brickmap-world.ts";
import { MapTiles, type Tile } from "./map-tiles.ts";
import { horizontalBounds, mapPoint, mapScreen } from "./map-camera.ts";

interface MapIndex { regions: RegionPosition[]; overview: RegionPosition[] }
interface SharedRenderer {
  domElement: HTMLCanvasElement;
  getContext(): WebGLRenderingContext | WebGL2RenderingContext;
  resetState(): void;
}
interface PendingSelection { ray: Ray; waiting?: string }
type Point = [number, number];
/** 一次手势开始时的地图相机和各手指位置；手指数变化时从当前位置重新开始，换手不会跳。 */
interface Gesture {
  button: number;
  camera: Camera;
  scale: number;
  start: Map<number, Point>;
  now: Map<number, Point>;
  moved: boolean;
}

// 地图底色与页面浅色界面一致；体素空隙和占用图背景都用它。
const BACKGROUND: Vec3 = [0.925, 0.933, 0.945];
const BACKGROUND_CSS = "#eceef1";
const MIN_SCALE = 0.125, MAX_SCALE = 128;
const FACES = ["西面", "东面", "底面", "顶面", "北面", "南面"];
const IDLE_READOUT = "点选地图上的方块";

export class Minimap {
  private readonly panel = document.getElementById("minimap") as HTMLElement;
  private readonly canvas = document.getElementById("minimap-canvas") as HTMLCanvasElement;
  private readonly context = this.canvas.getContext("2d")!;
  private readonly readout = document.getElementById("minimap-selection") as HTMLOutputElement;
  private readonly status = document.getElementById("minimap-status") as HTMLParagraphElement;
  private readonly zoom = document.getElementById("minimap-zoom") as HTMLInputElement;
  private readonly go = document.getElementById("minimap-jump") as HTMLButtonElement;
  private readonly tiles: MapTiles<ImageBitmap>;
  private readonly shared: SharedRenderer;
  private readonly jump: (position: Vec3) => void;
  private readonly inputChanged: () => void;
  private readonly focusScene: () => void;
  private world?: WorldInfo;
  private index?: MapIndex;
  private indexRequest?: AbortController;
  private scale = 0.75;
  private readonly camera: Camera = { target: [0, 80, 0], yaw: 0, pitch: 70, span: 150 };
  private player: Vec3 = [0, 80, 0];
  private yaw = 0;
  private selection?: Extract<PickResult, { kind: "hit" }>;
  private pending?: PendingSelection;
  private overview = false;
  private gesture?: Gesture;
  expanded = false;
  /** 用户隐藏界面时为 true。小地图画在共享 GL 画布上，CSS 隐藏不了，要靠它停画。 */
  concealed = false;

  constructor(
    shared: SharedRenderer,
    private readonly terrain: BrickmapWorld,
    jump: (position: Vec3) => void,
    inputChanged: () => void,
    focusScene: () => void,
  ) {
    // DOM 画布承接输入和标记；体素画在底下的共享 WebGL 画布中。
    // 收起时点一下只打开全屏地图，拖动、缩放和选点都在全屏地图里进行。
    this.shared = shared;
    this.jump = jump;
    this.inputChanged = inputChanged;
    this.focusScene = focusScene;
    this.tiles = new MapTiles(async (tile, signal) => {
      const query = new URLSearchParams({ world: String(this.world!.revision), kind: "overview", x: String(tile.x), z: String(tile.z) });
      const response = await fetch(`/api/minimap/tile?${query}`, { signal, cache: "no-store" });
      if (!response.ok) throw new Error((await response.text()).trim());
      return createImageBitmap(await response.blob());
    }, error => this.showError(error));
    document.getElementById("minimap-close")!.addEventListener("click", () => this.setExpanded(false));
    document.getElementById("minimap-retry")!.addEventListener("click", () => {
      this.terrain.downloads.retry();
      this.terrain.error = "";
      this.tiles.failed.clear();
      this.status.hidden = true;
      if (!this.index) this.loadIndex();
    });
    this.go.addEventListener("click", () => {
      const [x, y, z] = this.selection!.position;
      this.setExpanded(false);
      this.jump([x, y + 2, z]);
    });
    this.zoom.addEventListener("input", () => {
      const rect = this.canvas.getBoundingClientRect();
      this.zoomAt(rect.width / 2, rect.height / 2, 2 ** -Number(this.zoom.value));
    });
    this.panel.addEventListener("keydown", event => {
      if (event.key === "Escape" && this.expanded) { event.preventDefault(); this.setExpanded(false); }
    });
    this.canvas.addEventListener("click", () => {
      if (!this.expanded) this.setExpanded(true);
    });
    this.canvas.addEventListener("contextmenu", event => event.preventDefault());
    this.canvas.addEventListener("pointerdown", event => {
      if (!this.expanded || (event.button !== 0 && event.button !== 2)) return;
      event.preventDefault();
      this.inputChanged();
      this.canvas.focus();
      this.canvas.setPointerCapture(event.pointerId);
      const points = new Map(this.gesture?.now);
      points.set(event.pointerId, [event.clientX, event.clientY]);
      this.beginGesture(points, event.button, points.size > 1);
    });
    this.canvas.addEventListener("pointermove", event => {
      const gesture = this.gesture;
      if (!gesture?.now.has(event.pointerId)) return;
      gesture.now.set(event.pointerId, [event.clientX, event.clientY]);
      const [startX, startY] = gesture.start.get(event.pointerId)!;
      gesture.moved ||= Math.hypot(event.clientX - startX, event.clientY - startY) > 4;
      if (!gesture.moved) return;
      this.cancelPending();
      this.applyGesture(gesture);
    });
    const release = (event: PointerEvent): void => {
      const gesture = this.gesture;
      if (!gesture?.now.has(event.pointerId)) return;
      if (this.canvas.hasPointerCapture(event.pointerId)) this.canvas.releasePointerCapture(event.pointerId);
      const tapped = event.type === "pointerup" && !gesture.moved && gesture.now.size === 1 && gesture.button === 0;
      gesture.now.delete(event.pointerId);
      this.gesture = undefined;
      if (gesture.now.size) this.beginGesture(gesture.now, gesture.button, true);
      if (tapped) this.select(event.clientX, event.clientY);
    };
    this.canvas.addEventListener("pointerup", release);
    this.canvas.addEventListener("pointercancel", release);
    this.canvas.addEventListener("wheel", event => {
      event.preventDefault();
      if (!this.expanded) return;
      this.inputChanged();
      const rect = this.canvas.getBoundingClientRect();
      this.zoomAt(event.clientX - rect.left, event.clientY - rect.top,
        this.scale * Math.exp(Math.max(-1, Math.min(1, event.deltaY * 0.002))));
    }, { passive: false });
  }

  private beginGesture(points: Map<number, Point>, button: number, moved: boolean): void {
    this.gesture = {
      button,
      camera: { ...this.camera, target: [...this.camera.target] },
      scale: this.scale,
      start: new Map(points),
      now: new Map(points),
      moved,
    };
  }

  private applyGesture(gesture: Gesture): void {
    // 单指或左键平移，右键旋转；两指同时旋转、缩放，并让两指中点下的地面跟住手指。
    const rect = this.canvas.getBoundingClientRect();
    const local = ([x, y]: Point): Point => [x - rect.left, y - rect.top];
    const { start, now } = gesture;
    const ids = [...start.keys()];
    if (ids.length === 1 && gesture.button === 2) {
      const [x0, y0] = start.get(ids[0])!, [x, y] = now.get(ids[0])!;
      this.camera.yaw = gesture.camera.yaw - (x - x0) * 0.3;
      this.camera.pitch = Math.max(40, Math.min(85, gesture.camera.pitch + (y - y0) * 0.25));
      return;
    }
    let from = local(start.get(ids[0])!), to = local(now.get(ids[0])!);
    this.camera.yaw = gesture.camera.yaw;
    this.scale = gesture.scale;
    if (ids.length > 1) {
      const a0 = local(start.get(ids[0])!), b0 = local(start.get(ids[1])!);
      const a = local(now.get(ids[0])!), b = local(now.get(ids[1])!);
      const angle = (p: Point, q: Point): number => Math.atan2(q[1] - p[1], q[0] - p[0]);
      this.camera.yaw = gesture.camera.yaw + (angle(a, b) - angle(a0, b0)) * 180 / Math.PI;
      this.scale = this.clampScale(gesture.scale * Math.hypot(b0[0] - a0[0], b0[1] - a0[1]) / Math.max(1, Math.hypot(b[0] - a[0], b[1] - a[1])));
      from = [(a0[0] + b0[0]) / 2, (a0[1] + b0[1]) / 2];
      to = [(a[0] + b[0]) / 2, (a[1] + b[1]) / 2];
    }
    this.camera.span = rect.height * this.scale;
    const anchor = mapPoint(gesture.camera, rect.width, rect.height, ...from);
    this.camera.target = [...gesture.camera.target];
    const moved = mapPoint(this.camera, rect.width, rect.height, ...to);
    this.camera.target = gesture.camera.target.map((n, i) => n + anchor[i] - moved[i]) as Vec3;
    this.syncZoom();
  }

  private clampScale(scale: number): number {
    return Math.max(MIN_SCALE, Math.min(MAX_SCALE, scale));
  }

  private zoomAt(x: number, y: number, scale: number): void {
    // 缩放时光标（或滑条缩放时的画面中心）下的地面保持不动。
    const rect = this.canvas.getBoundingClientRect();
    const anchor = mapPoint(this.camera, rect.width, rect.height, x, y);
    this.scale = this.clampScale(scale);
    this.camera.span = rect.height * this.scale;
    const current = mapPoint(this.camera, rect.width, rect.height, x, y);
    this.camera.target = this.camera.target.map((n, i) => n + anchor[i] - current[i]) as Vec3;
    this.cancelPending();
    this.syncZoom();
  }

  private syncZoom(): void {
    // 滑条向右是放大，所以取每像素方块数的负对数。
    this.zoom.value = String(-Math.log2(this.scale));
  }

  private cancelPending(): void {
    if (!this.pending) return;
    this.pending = undefined;
    this.readout.value = IDLE_READOUT;
  }

  setWorld(world: WorldInfo): void {
    // 异步回调只改变 CPU 状态；旧 GPU 资源在下一帧统一释放和重建。
    this.setExpanded(false);
    this.world = world;
    this.indexRequest?.abort();
    this.index = undefined;
    this.tiles.reset();
    this.player = [...world.position];
    this.camera.target = [...world.position];
    this.camera.yaw = 0;
    this.camera.pitch = 70;
    this.scale = 0.75;
    this.syncZoom();
    this.selection = undefined;
    this.pending = undefined;
    this.gesture = undefined;
    this.status.hidden = true;
    this.readout.value = IDLE_READOUT;
    this.go.disabled = true;
    this.panel.hidden = !world.minimap;
    if (world.minimap) this.loadIndex();
  }

  private loadIndex(): void {
    // 索引只用于占用图，体素区域请求无需等待全世界坐标扫描。
    this.indexRequest?.abort();
    const controller = this.indexRequest = new AbortController();
    void fetch(`/api/minimap/index?world=${this.world!.revision}`, { signal: controller.signal, cache: "no-store" })
      .then(async response => {
        if (!response.ok) throw new Error((await response.text()).trim());
        const index: MapIndex = await response.json();
        if (!controller.signal.aborted) this.index = index;
      }).catch((error: unknown) => { if (!controller.signal.aborted) this.showError(error); });
  }

  /** 打开或关闭全屏地图。打开时三维操作暂停，关闭后小地图重新跟随玩家。 */
  setExpanded(expanded: boolean): void {
    if (expanded && this.panel.hidden) return;
    this.expanded = expanded;
    this.panel.dataset.expanded = String(expanded);
    document.body.classList.toggle("minimap-expanded", expanded);
    this.gesture = undefined;
    this.inputChanged();
    if (expanded) this.canvas.focus();
    else this.focusScene();
  }

  private select(clientX: number, clientY: number): void {
    // 占用图先定位并放大；实际高度始终来自区域到达后的体素射线。
    const rect = this.canvas.getBoundingClientRect();
    const x = clientX - rect.left, y = clientY - rect.top;
    let ray = cameraRay(this.camera, rect.width, rect.height, x, y);
    const point = mapPoint(this.camera, rect.width, rect.height, x, y);
    const origin = windowOrigin(this.camera.target[0], this.camera.target[2]);
    const outside = point[0] < origin.x * REGION_SIZE || point[2] < origin.z * REGION_SIZE
      || point[0] >= origin.x * REGION_SIZE + WINDOW_SIZE || point[2] >= origin.z * REGION_SIZE + WINDOW_SIZE;
    // 缩远后可以点到没有预览的边缘，先把数据窗口移到该水平位置再拾取。
    if (this.overview || outside) {
      this.camera.target = point;
      if (this.overview) {
        this.scale = 0.25;
        this.syncZoom();
      }
      ray = { origin: [...point], direction: cameraBasis(this.camera).direction };
    }
    const top = this.world!.minimapBounds![0] + this.world!.minimapBounds![1] + 1;
    const t = (top - ray.origin[1]) / ray.direction[1];
    ray.origin = ray.origin.map((n, i) => n + ray.direction[i] * t) as Vec3;
    this.pending = { ray };
    this.selection = undefined;
    this.go.disabled = true;
    this.readout.value = "正在定位…";
  }

  private showError(error: unknown): void {
    this.status.firstElementChild!.textContent = `地图加载失败：${error instanceof Error ? error.message : String(error)}`;
    this.status.hidden = false;
  }

  update(x: number, y: number, z: number, yaw: number): void {
    // 先确定地图中心，两个窗口的上传统一由主帧循环调度。
    if (!this.expanded && !this.pending && (x !== this.player[0] || z !== this.player[2])) {
      this.camera.target[0] = x;
      this.camera.target[2] = z;
    }
    this.player = [x, y, z];
    this.yaw = yaw;
    this.camera.target[1] = y;
    this.terrain.map.center = [this.camera.target[0], this.camera.target[2]];
  }

  render(): void {
    // 主场景清屏后每帧重画小地图，绘制和拾取共享已发布的窗口目录。
    const gpu = this.terrain.map.gpu;
    if (!gpu || this.panel.hidden || (this.concealed && !this.expanded)) return;
    try {
      const rect = this.canvas.getBoundingClientRect();
      this.camera.span = rect.height * this.scale;
      const full = this.terrain.full(this.terrain.map);
      // 只按缩放和显示尺寸切换，旋转时允许窗口边缘缺少体素预览。
      this.overview = full || Math.max(rect.width, rect.height) * this.scale > WINDOW_SIZE;
      if (this.pending && !full) this.pick();
      if (!this.overview) {
        const canvasRect = this.shared.domElement.getBoundingClientRect();
        const gl = gpu.draw.gl;
        const rx = gl.drawingBufferWidth / canvasRect.width, ry = gl.drawingBufferHeight / canvasRect.height;
        const left = Math.round((rect.left - canvasRect.left) * rx);
        const bottom = Math.round((canvasRect.bottom - rect.bottom) * ry);
        const right = Math.round((rect.right - canvasRect.left) * rx);
        const top = Math.round((canvasRect.bottom - rect.top) * ry);
        gpu.draw.draw(this.camera, {
          viewport: [left, bottom, right - left, top - bottom],
          openBoundary: true,
          timing: false,
          background: BACKGROUND,
        });
      }
      this.drawOverlay(rect.width, rect.height);
      if (this.terrain.error) this.showError(this.terrain.error);
      else if (full) {
        this.status.firstElementChild!.textContent = "体素显存已满，暂时显示区块分布；移动到较简单的区域后可重试。";
        this.status.hidden = false;
      }
    } finally {
      this.shared.resetState();
    }
  }

  private pick(): void {
    // 遇到未知区域就继续等待，只有走完已知体积的射线才报告未命中。
    const pending = this.pending!;
    if (pending.waiting && this.terrain.downloads.entries.get(pending.waiting)?.state !== "resident") return;
    const result = this.terrain.map.gpu!.draw.pick(pending.ray, { openBoundary: true });
    if (result.kind === "missing") {
      pending.waiting = regionKey({ x: Math.floor(result.position[0] / REGION_SIZE), z: Math.floor(result.position[2] / REGION_SIZE) });
      this.camera.target[0] = result.position[0];
      this.camera.target[2] = result.position[2];
      this.readout.value = "正在载入选点所在区域…";
      return;
    }
    this.pending = undefined;
    if (result.kind === "miss") {
      this.readout.value = "这里没有方块";
      return;
    }
    this.selection = result;
    const [x, y, z] = result.position;
    this.readout.value = `X ${x}  Y ${y}  Z ${z} · ${FACES[result.face]}`;
    this.go.disabled = false;
  }

  private drawOverlay(width: number, height: number): void {
    // 透明标记画布与 GL 视口重合，占用图模式才覆盖底色。
    const ratio = Math.min(devicePixelRatio, 2);
    const w = Math.round(width * ratio), h = Math.round(height * ratio);
    if (this.canvas.width !== w || this.canvas.height !== h) { this.canvas.width = w; this.canvas.height = h; }
    const ctx = this.context;
    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    ctx.clearRect(0, 0, width, height);
    if (this.overview) {
      ctx.fillStyle = BACKGROUND_CSS;
      ctx.fillRect(0, 0, width, height);
      const bounds = horizontalBounds(this.camera, width, height, this.camera.target[1], this.camera.target[1]);
      const tiles: Tile[] = (this.index?.overview ?? [])
        .filter(p => (p.x + 1) * 2048 >= bounds[0] && p.x * 2048 <= bounds[2] && (p.z + 1) * 2048 >= bounds[1] && p.z * 2048 <= bounds[3])
        .map(p => ({ ...p, kind: "overview", key: `overview/${p.x}/${p.z}` }));
      this.tiles.update(tiles);
      const { right, up } = cameraBasis(this.camera);
      const factor = height / this.camera.span;
      const zero = mapScreen(this.camera, width, height, [0, this.camera.target[1], 0]);
      ctx.save();
      ctx.transform(right[0] * factor, -up[0] * factor, right[2] * factor, -up[2] * factor, zero[0], zero[1]);
      ctx.imageSmoothingEnabled = false;
      for (const tile of tiles) {
        const image = this.tiles.images.get(tile.key);
        if (image) ctx.drawImage(image, tile.x * 2048, tile.z * 2048, 2048, 2048);
      }
      ctx.restore();
    } else this.tiles.update([]);
    if (this.selection && this.expanded) {
      const [sx, sy] = mapScreen(this.camera, width, height, this.selection.position.map(n => n + 0.5) as Vec3);
      ctx.strokeStyle = "#1d1f23";
      ctx.lineWidth = 1.5;
      ctx.beginPath();
      ctx.arc(sx, sy, 8, 0, Math.PI * 2);
      ctx.moveTo(sx - 13, sy); ctx.lineTo(sx + 13, sy);
      ctx.moveTo(sx, sy - 13); ctx.lineTo(sx, sy + 13);
      ctx.stroke();
    }
    const [px, py] = mapScreen(this.camera, width, height, this.player);
    const ahead = mapScreen(this.camera, width, height, [this.player[0] - Math.sin(this.yaw), this.player[1], this.player[2] - Math.cos(this.yaw)]);
    ctx.save();
    ctx.translate(px, py);
    ctx.rotate(Math.atan2(ahead[0] - px, py - ahead[1]));
    ctx.fillStyle = "#ffffff"; ctx.strokeStyle = "#1d1f23"; ctx.lineWidth = 1.5; ctx.lineJoin = "round";
    ctx.beginPath(); ctx.moveTo(0, -9); ctx.lineTo(6, 7); ctx.lineTo(0, 4); ctx.lineTo(-6, 7); ctx.closePath();
    ctx.fill(); ctx.stroke(); ctx.restore();
  }

  dispose(): void {
    this.indexRequest?.abort();
    this.tiles.reset();
    this.panel.hidden = true;
  }
}
