import type { WorldInfo } from "./picker.ts";
import { BrickmapPool, BrickmapWindow, type GPURegion } from "../../../../../../neomega-viewer-core/viewer/brickmap/web/gpu.ts";
import { BrickmapRenderer, cameraRay, type Ray, type PickResult } from "../../../../../../neomega-viewer-core/viewer/brickmap/web/renderer.ts";
import { cameraBasis, type Camera, type Vec3 } from "../../../../../../neomega-viewer-core/viewer/brickmap/web/volume.ts";
import { RegionDownloads, regionKey, windowOrigin, type RegionPosition } from "./map-regions.ts";
import { MapTiles, type Tile } from "./map-tiles.ts";
import { horizontalBounds, mapPoint, mapScreen } from "./map-camera.ts";

interface MapIndex { regions: RegionPosition[]; overview: RegionPosition[] }
interface SharedRenderer {
  domElement: HTMLCanvasElement;
  getContext(): WebGLRenderingContext | WebGL2RenderingContext;
  resetState(): void;
}
interface PendingSelection { ray: Ray; waiting?: string }
interface GPUMap {
  pool: BrickmapPool;
  window: BrickmapWindow;
  draw: BrickmapRenderer;
  regions: Map<string, GPURegion>;
}

export class Minimap {
  private readonly panel = document.getElementById("minimap") as HTMLElement;
  private readonly canvas = document.getElementById("minimap-canvas") as HTMLCanvasElement;
  private readonly context = this.canvas.getContext("2d")!;
  private readonly readout = document.getElementById("minimap-selection") as HTMLOutputElement;
  private readonly status = document.getElementById("minimap-status") as HTMLOutputElement;
  private readonly scaleLabel = document.getElementById("minimap-scale") as HTMLOutputElement;
  private readonly go = document.getElementById("minimap-jump") as HTMLButtonElement;
  private readonly tiles: MapTiles<ImageBitmap>;
  private readonly downloads: RegionDownloads;
  private readonly shared: SharedRenderer;
  private readonly jump: (position: Vec3) => void;
  private readonly inputChanged: () => void;
  private readonly focusScene: () => void;
  private world?: WorldInfo;
  private index?: MapIndex;
  private indexRequest?: AbortController;
  private gpu?: GPUMap;
  private replaceGPU = false;
  private scale = 0.75;
  private readonly camera: Camera = { target: [0, 80, 0], yaw: 0, pitch: 70, span: 150 };
  private player: Vec3 = [0, 80, 0];
  private yaw = 0;
  private selection?: Extract<PickResult, { kind: "hit" }>;
  private pending?: PendingSelection;
  private overview = false;
  private pointer?: {
    id: number; button: number; x: number; y: number; camera: Camera; anchor: Vec3; moved: boolean;
  };
  expanded = false;

  constructor(
    shared: SharedRenderer,
    jump: (position: Vec3) => void,
    inputChanged: () => void,
    focusScene: () => void,
  ) {
    // DOM 画布承接输入、标记和占用图；体素画在底下的共享 WebGL 画布中。
    this.shared = shared;
    this.jump = jump;
    this.inputChanged = inputChanged;
    this.focusScene = focusScene;
    this.downloads = new RegionDownloads(error => this.showError(error));
    this.tiles = new MapTiles(async (tile, signal) => {
      const query = new URLSearchParams({ world: String(this.world!.revision), kind: "overview", x: String(tile.x), z: String(tile.z) });
      const response = await fetch(`/api/minimap/tile?${query}`, { signal, cache: "no-store" });
      if (!response.ok) throw new Error((await response.text()).trim());
      return createImageBitmap(await response.blob());
    }, error => this.showError(error));
    document.getElementById("minimap-expand")!.addEventListener("click", () => this.setExpanded(true));
    document.getElementById("minimap-close")!.addEventListener("click", () => this.setExpanded(false));
    document.getElementById("minimap-retry")!.addEventListener("click", () => {
      this.downloads.retry();
      this.tiles.failed.clear();
      this.status.value = "";
      if (!this.index) this.loadIndex();
    });
    this.go.addEventListener("click", () => {
      const [x, y, z] = this.selection!.position;
      this.setExpanded(false);
      this.jump([x, y + 2, z]);
    });
    this.panel.addEventListener("keydown", event => {
      if (event.key === "Escape") { event.preventDefault(); this.setExpanded(false); }
      if (event.target === this.canvas && (event.key === "Enter" || event.key === " ")) {
        event.preventDefault(); this.setExpanded(!this.expanded);
      }
    });
    this.canvas.addEventListener("contextmenu", event => event.preventDefault());
    this.canvas.addEventListener("pointerdown", event => {
      if (event.button !== 0 && event.button !== 2) return;
      event.preventDefault();
      this.inputChanged();
      this.canvas.focus();
      const rect = this.canvas.getBoundingClientRect();
      this.pointer = {
        id: event.pointerId, button: event.button, x: event.clientX, y: event.clientY,
        camera: { ...this.camera, target: [...this.camera.target] },
        anchor: mapPoint(this.camera, rect.width, rect.height, event.clientX - rect.left, event.clientY - rect.top),
        moved: false,
      };
      this.canvas.setPointerCapture(event.pointerId);
    });
    this.canvas.addEventListener("pointermove", event => {
      const pointer = this.pointer;
      if (!pointer || pointer.id !== event.pointerId) return;
      const dx = event.clientX - pointer.x, dy = event.clientY - pointer.y;
      pointer.moved ||= Math.hypot(dx, dy) > 4;
      if (!pointer.moved) return;
      if (this.pending) {
        this.pending = undefined;
        this.readout.value = "点选地图查看方块坐标";
      }
      if (pointer.button === 2) {
        this.camera.yaw = pointer.camera.yaw - dx * 0.3;
        this.camera.pitch = Math.max(40, Math.min(85, pointer.camera.pitch + dy * 0.25));
      } else {
        const rect = this.canvas.getBoundingClientRect();
        const current = mapPoint(pointer.camera, rect.width, rect.height, event.clientX - rect.left, event.clientY - rect.top);
        this.camera.target = pointer.camera.target.map((n, i) => n + pointer.anchor[i] - current[i]) as Vec3;
      }
    });
    this.canvas.addEventListener("pointerup", event => {
      const pointer = this.pointer;
      this.pointer = undefined;
      if (!pointer || pointer.id !== event.pointerId) return;
      if (this.canvas.hasPointerCapture(event.pointerId)) this.canvas.releasePointerCapture(event.pointerId);
      if (!pointer.moved && pointer.button === 0) this.select(event.clientX, event.clientY);
    });
    this.canvas.addEventListener("pointercancel", () => { this.pointer = undefined; });
    this.canvas.addEventListener("wheel", event => {
      event.preventDefault();
      this.inputChanged();
      const rect = this.canvas.getBoundingClientRect();
      const x = event.clientX - rect.left, y = event.clientY - rect.top;
      const anchor = mapPoint(this.camera, rect.width, rect.height, x, y);
      this.scale = Math.max(0.125, Math.min(128, this.scale * Math.exp(Math.max(-1, Math.min(1, event.deltaY * 0.002)))));
      this.camera.span = rect.height * this.scale;
      const current = mapPoint(this.camera, rect.width, rect.height, x, y);
      this.camera.target = this.camera.target.map((n, i) => n + anchor[i] - current[i]) as Vec3;
      if (this.pending) {
        this.pending = undefined;
        this.readout.value = "点选地图查看方块坐标";
      }
    }, { passive: false });
  }

  setWorld(world: WorldInfo): void {
    // 异步回调只改变 CPU 状态；旧 GPU 资源在下一帧统一释放和重建。
    this.setExpanded(false);
    this.world = world;
    this.indexRequest?.abort();
    this.index = undefined;
    this.downloads.reset(world.revision);
    this.tiles.reset();
    this.replaceGPU = true;
    this.player = [...world.position];
    this.camera.target = [...world.position];
    this.camera.yaw = 0;
    this.camera.pitch = 70;
    this.scale = 0.75;
    this.selection = undefined;
    this.pending = undefined;
    this.pointer = undefined;
    this.status.value = "";
    this.readout.value = "点选地图查看方块坐标";
    this.go.disabled = true;
    this.panel.hidden = !world.minimap;
    document.body.classList.toggle("has-minimap", Boolean(world.minimap));
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

  private setExpanded(expanded: boolean): void {
    // 普通覆盖层留出透明地图区域，主画布不会被 dialog 顶层遮罩压暗。
    this.expanded = expanded;
    this.panel.dataset.expanded = String(expanded);
    document.body.classList.toggle("minimap-expanded", expanded);
    this.pointer = undefined;
    this.inputChanged();
    if (expanded) this.canvas.focus();
    else this.focusScene();
  }

  private select(clientX: number, clientY: number): void {
    // 占用图先定位并放大；实际高度始终来自区域到达后的体素射线。
    const rect = this.canvas.getBoundingClientRect();
    const x = clientX - rect.left, y = clientY - rect.top;
    let ray = cameraRay(this.camera, rect.width, rect.height, x, y);
    if (this.overview) {
      this.camera.target = mapPoint(this.camera, rect.width, rect.height, x, y);
      this.scale = 0.25;
      ray = { origin: [...this.camera.target], direction: cameraBasis(this.camera).direction };
    }
    const top = this.world!.minimapBounds![0] + this.world!.minimapBounds![1] + 1;
    const t = (top - ray.origin[1]) / ray.direction[1];
    ray.origin = ray.origin.map((n, i) => n + ray.direction[i] * t) as Vec3;
    this.pending = { ray };
    this.selection = undefined;
    this.go.disabled = true;
    this.readout.value = "正在定位方块…";
    this.setExpanded(true);
  }

  private showError(error: unknown): void {
    this.status.value = `地图加载失败：${error instanceof Error ? error.message : String(error)}。可重试。`;
  }

  update(x: number, y: number, z: number, yaw: number): void {
    // 必须在主场景之后每帧调用，主场景清屏会覆盖上一帧的小地图。
    try {
      if (this.replaceGPU) {
        this.disposeGPU();
        this.replaceGPU = false;
        if (this.world?.minimap) {
          const gl = this.shared.getContext() as WebGL2RenderingContext;
          const [minY, height] = this.world.minimapBounds!;
          const origin = windowOrigin(this.camera.target[0], this.camera.target[2]);
          const pool = new BrickmapPool(gl);
          const window = new BrickmapWindow(gl, [origin.x * 128, minY, origin.z * 128], [1024, height, 1024]);
          this.gpu = { pool, window, draw: new BrickmapRenderer(gl, pool, window), regions: new Map() };
        }
      }
      if (!this.gpu || this.panel.hidden) return;
      if (!this.expanded && !this.pointer && !this.pending && (x !== this.player[0] || z !== this.player[2])) {
        this.camera.target[0] = x;
        this.camera.target[2] = z;
      }
      this.player = [x, y, z];
      this.yaw = yaw;
      this.camera.target[1] = y;
      this.updateRegions();
      const rect = this.canvas.getBoundingClientRect();
      this.camera.span = rect.height * this.scale;
      const [minY, height] = this.world!.minimapBounds!;
      const bounds = horizontalBounds(this.camera, rect.width, rect.height, minY, minY + height);
      const origin = this.gpu.window.origin;
      const full = [...this.downloads.entries.values()].some(entry => entry.state === "full");
      this.overview = full || bounds[0] < origin[0] || bounds[1] < origin[2]
        || bounds[2] > origin[0] + 1024 || bounds[3] > origin[2] + 1024;
      this.panel.dataset.mode = this.overview ? "overview" : "brickmap";
      if (this.pending && !full) this.pick();
      if (!this.overview) {
        const canvasRect = this.shared.domElement.getBoundingClientRect();
        const gl = this.gpu.draw.gl;
        const rx = gl.drawingBufferWidth / canvasRect.width, ry = gl.drawingBufferHeight / canvasRect.height;
        const left = Math.round((rect.left - canvasRect.left) * rx);
        const bottom = Math.round((canvasRect.bottom - rect.bottom) * ry);
        const right = Math.round((rect.right - canvasRect.left) * rx);
        const top = Math.round((canvasRect.bottom - rect.top) * ry);
        this.gpu.draw.draw(this.camera, { viewport: [left, bottom, right - left, top - bottom], openBoundary: true, timing: false });
      }
      this.drawOverlay(rect.width, rect.height);
      const queued = [...this.downloads.entries.values()].filter(entry => entry.state !== "resident").length;
      const mode = this.overview ? "区块分布" : `${this.scale.toFixed(2)} 方块 / px`;
      this.scaleLabel.value = `${mode}${queued ? ` · ${queued} 区域待载入` : ""}`;
      if (full) this.status.value = "体素显存预算已满，显示区块分布；移动到较简单的区域后可重试。";
    } finally {
      this.shared.resetState();
    }
  }

  private updateRegions(): void {
    // 窗口先摘除离开的引用，再回收槽位，最后安装下载完成的区域。
    const gpu = this.gpu!;
    const origin = windowOrigin(this.camera.target[0], this.camera.target[2]);
    const inside = (position: RegionPosition): boolean => position.x >= origin.x && position.x < origin.x + 8
      && position.z >= origin.z && position.z < origin.z + 8;
    const wanted: RegionPosition[] = [];
    for (let z = origin.z; z < origin.z + 8; z++) for (let x = origin.x; x < origin.x + 8; x++) wanted.push({ x, z });
    const priority = (position: RegionPosition): number => {
      const key = regionKey(position);
      if (key === this.pending?.waiting) return -1;
      return ((position.x + 0.5) * 128 - this.camera.target[0]) ** 2
        + ((position.z + 0.5) * 128 - this.camera.target[2]) ** 2;
    };
    wanted.sort((a, b) => priority(a) - priority(b));
    const keys = new Set(wanted.map(regionKey));
    const evicted = [...gpu.regions].filter(([key]) => !keys.has(key));
    const moved = origin.x * 128 !== gpu.window.origin[0] || origin.z * 128 !== gpu.window.origin[2];
    if (moved) {
      const retained = [...gpu.regions.values()].filter(region => inside({ x: region.origin[0] / 128, z: region.origin[2] / 128 }));
      gpu.window.rebuild([origin.x * 128, gpu.window.origin[1], origin.z * 128], retained);
    }
    for (const [key, region] of evicted) {
      gpu.pool.release(region);
      gpu.regions.delete(key);
    }
    this.downloads.update(wanted);
    // 每帧只处理一个区域，响应积压不会在一次动画帧中连续上传整片窗口。
    const request = wanted.map(p => this.downloads.entries.get(regionKey(p))!).find(entry => entry.state === "ready");
    if (request) {
      const region = gpu.pool.upload(request.volume!);
      request.volume = undefined;
      request.state = region ? "resident" : "full";
      if (region) {
        gpu.regions.set(request.key, region);
        if (inside(request)) gpu.window.install(region);
      }
    }

  }

  private pick(): void {
    // 遇到未知区域就继续等待，只有走完已知体积的射线才报告未命中。
    const pending = this.pending!;
    if (pending.waiting && this.downloads.entries.get(pending.waiting)?.state !== "resident") return;
    const result = this.gpu!.draw.pick(pending.ray, { openBoundary: true });
    if (result.kind === "missing") {
      pending.waiting = regionKey({ x: Math.floor(result.position[0] / 128), z: Math.floor(result.position[2] / 128) });
      this.camera.target[0] = result.position[0];
      this.camera.target[2] = result.position[2];
      this.readout.value = "正在载入选点所在区域…";
      return;
    }
    this.pending = undefined;
    if (result.kind === "miss") {
      this.readout.value = "这条射线没有命中方块";
      return;
    }
    this.selection = result;
    const [x, y, z] = result.position;
    this.readout.value = `X ${x} · Y ${y} · Z ${z} · ${["西面", "东面", "底面", "顶面", "北面", "南面"][result.face]}`;
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
      ctx.fillStyle = "#0c1018";
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
    if (this.selection) {
      const [sx, sy] = mapScreen(this.camera, width, height, this.selection.position.map(n => n + 0.5) as Vec3);
      ctx.strokeStyle = "#ffc98a";
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
    ctx.fillStyle = "#f7e8d2"; ctx.strokeStyle = "#2b2119"; ctx.lineWidth = 2;
    ctx.beginPath(); ctx.moveTo(0, -10); ctx.lineTo(6, 7); ctx.lineTo(0, 4); ctx.lineTo(-6, 7); ctx.closePath();
    ctx.stroke(); ctx.fill(); ctx.restore();
    ctx.fillStyle = "#f7e8d2"; ctx.font = "11px ui-monospace, monospace";
    ctx.fillText(`N · ${Math.round(((this.camera.yaw % 360) + 360) % 360)}°`, 10, 18);
  }

  private disposeGPU(): void {
    // 所有目录和绘制器结束使用后，一次释放共享纹理。
    if (!this.gpu) return;
    this.gpu.draw.dispose();
    this.gpu.window.dispose();
    this.gpu.pool.dispose();
    this.gpu = undefined;
  }

  dispose(): void {
    this.indexRequest?.abort();
    this.downloads.reset(0);
    this.tiles.reset();
    this.disposeGPU();
    this.panel.hidden = true;
  }
}
