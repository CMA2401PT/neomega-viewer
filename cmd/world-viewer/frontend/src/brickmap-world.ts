import { BrickmapPool, BrickmapWindow, type GPURegion } from "../../../../../../neomega-viewer-core/viewer/brickmap/web/gpu.ts";
import { BrickmapRenderer } from "../../../../../../neomega-viewer-core/viewer/brickmap/web/renderer.ts";
import type { WorldInfo } from "./picker.ts";
import { REGION_SIZE, WINDOW_REGIONS, WINDOW_SIZE, RegionDownloads, regionKey, windowOrigin, type RegionPosition } from "./map-regions.ts";

export class StreamedBrickmap {
  center: [number, number] = [0, 0];
  enabled = false;
  positions: RegionPosition[] = [];
  gpu?: { window: BrickmapWindow; draw: BrickmapRenderer };

  contains(position: RegionPosition): boolean {
    const origin = windowOrigin(...this.center);
    return position.x >= origin.x && position.x < origin.x + WINDOW_REGIONS
      && position.z >= origin.z && position.z < origin.z + WINDOW_REGIONS;
  }

  dispose(): void {
    this.gpu?.draw.dispose();
    this.gpu?.window.dispose();
    this.gpu = undefined;
    this.positions = [];
  }
}

export class BrickmapWorld {
  readonly map = new StreamedBrickmap();
  readonly far = new StreamedBrickmap();
  readonly downloads: RegionDownloads;
  error = "";
  private pool?: BrickmapPool;
  private world?: WorldInfo;
  private replaceGPU = false;
  private readonly regions = new Map<string, GPURegion>();

  constructor(private readonly gl: WebGL2RenderingContext) {
    // 两个窗口共用下载、材质和砖池；每个窗口只持有自己的目录与相机。
    this.downloads = new RegionDownloads(error => { this.error = String(error); });
  }

  setWorld(world: WorldInfo): void {
    // 取消属于旧存档的响应，GPU 资源统一留到下一帧释放。
    this.world = world;
    this.downloads.reset(world.revision);
    this.error = "";
    this.replaceGPU = true;
    this.map.enabled = Boolean(world.minimap);
    this.far.enabled = false;
    this.map.center = this.far.center = [world.position[0], world.position[2]];
  }

  full(view: StreamedBrickmap): boolean {
    return view.positions.some(p => this.downloads.entries.get(regionKey(p))?.state === "full");
  }

  pending(view: StreamedBrickmap): number {
    return view.positions.filter(p => this.downloads.entries.get(regionKey(p))?.state !== "resident").length;
  }

  update(): void {
    // 所有目录先摘除离开的引用，再回收槽位；完成上传后才向各窗口发布。
    if (this.replaceGPU) {
      this.disposeGPU();
      if (this.world?.minimap) this.pool = new BrickmapPool(this.gl);
      this.replaceGPU = false;
    }
    if (!this.pool) return;
    const active = [this.map, this.far].filter(view => view.enabled);
    const wanted = new Map<string, RegionPosition>();
    for (const view of [this.map, this.far]) {
      if (!view.enabled) { view.dispose(); continue; }
      const origin = windowOrigin(...view.center);
      view.positions = [];
      for (let z = origin.z; z < origin.z + WINDOW_REGIONS; z++) {
        for (let x = origin.x; x < origin.x + WINDOW_REGIONS; x++) {
          const position = { x, z };
          view.positions.push(position);
          wanted.set(regionKey(position), position);
        }
      }
      const [minY, height] = this.world!.minimapBounds!;
      const nextOrigin: [number, number, number] = [origin.x * REGION_SIZE, minY, origin.z * REGION_SIZE];
      const created = !view.gpu;
      if (created) {
        const window = new BrickmapWindow(this.gl, nextOrigin, [WINDOW_SIZE, height, WINDOW_SIZE]);
        view.gpu = { window, draw: new BrickmapRenderer(this.gl, this.pool, window) };
      }
      const { window } = view.gpu!;
      if (created || window.origin[0] !== nextOrigin[0] || window.origin[2] !== nextOrigin[2]) {
        const retained = [...this.regions.values()].filter(region => view.contains({
          x: region.origin[0] / REGION_SIZE,
          z: region.origin[2] / REGION_SIZE,
        }));
        window.rebuild(nextOrigin, retained);
      }
    }
    for (const [key, region] of this.regions) {
      if (wanted.has(key)) continue;
      this.pool.release(region);
      this.regions.delete(key);
    }
    const distance = (p: RegionPosition): number => Math.min(...active.map(view =>
      ((p.x + 0.5) * REGION_SIZE - view.center[0]) ** 2
        + ((p.z + 0.5) * REGION_SIZE - view.center[1]) ** 2));
    const ordered = [...wanted.values()].sort((a, b) => distance(a) - distance(b));
    this.downloads.update(ordered);
    const request = ordered.map(p => this.downloads.entries.get(regionKey(p))!).find(entry => entry.state === "ready");
    if (!request) return;
    const region = this.pool.upload(request.volume!);
    request.volume = undefined;
    request.state = region ? "resident" : "full";
    if (!region) return;
    this.regions.set(request.key, region);
    for (const view of active) if (view.contains(request)) view.gpu!.window.install(region);
  }

  private disposeGPU(): void {
    this.map.dispose();
    this.far.dispose();
    this.regions.clear();
    this.pool?.dispose();
    this.pool = undefined;
  }

  dispose(): void {
    this.downloads.reset(0);
    this.disposeGPU();
  }
}
