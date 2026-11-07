import type { WorldInfo } from "./picker.ts";

type Point = { x: number; z: number };
type Kind = "detail" | "overview";
type Tile = Point & { kind: Kind; key: string };
interface MapIndex {
  detail: Point[];
  overview: Point[];
}
interface Selection extends Point {
  y?: number | null;
  pending: boolean;
}

export function mapPoint(
  center: Point,
  scale: number,
  width: number,
  height: number,
  x: number,
  y: number,
): Point {
  // 画布与世界统一采用北上南下，缩放锚点和点选使用同一坐标变换。
  return {
    x: center.x + (x - width / 2) * scale,
    z: center.z + (y - height / 2) * scale,
  };
}

export function visibleTiles(
  center: Point,
  scale: number,
  width: number,
  height: number,
  index?: MapIndex,
): Tile[] {
  // 远景限制方块图块的数量，世界再大也只读取占用图。
  const bounds = (size: number): [number, number, number, number] => [
    Math.floor((center.x - width * scale / 2) / size),
    Math.floor((center.x + width * scale / 2) / size),
    Math.floor((center.z - height * scale / 2) / size),
    Math.floor((center.z + height * scale / 2) / size),
  ];
  const detail = bounds(128);
  const detailCount = (detail[1] - detail[0] + 1) * (detail[3] - detail[2] + 1);
  const kind: Kind = scale > 4 || detailCount > 192 ? "overview" : "detail";
  const size = kind === "detail" ? 128 : 2048;
  const [minX, maxX, minZ, maxZ] = bounds(size);
  const positions: Point[] = index?.[kind] ?? [];
  if (!index && kind === "detail") {
    for (let z = minZ; z <= maxZ; z++) {
      for (let x = minX; x <= maxX; x++) positions.push({ x, z });
    }
  }
  return positions
    .filter(({ x, z }) => x >= minX && x <= maxX && z >= minZ && z <= maxZ)
    .map(({ x, z }) => ({ x, z, kind, key: `${kind}/${x}/${z}` }))
    .sort((a, b) =>
      ((a.x + 0.5) * size - center.x) ** 2
      + ((a.z + 0.5) * size - center.z) ** 2
      - ((b.x + 0.5) * size - center.x) ** 2
      - ((b.z + 0.5) * size - center.z) ** 2,
    );
}

export class MapTiles<T extends { close(): void }> {
  // 两种视图共享图片；废弃请求占着名额直到真正返回，切换代次后迟到的图片立即释放。
  readonly images = new Map<string, T>();
  readonly failed = new Set<string>();
  private wanted: Tile[] = [];
  private active = new Map<string, AbortController>();
  private epoch = 0;
  private readonly load: (tile: Tile, signal: AbortSignal) => Promise<T>;
  private readonly changed: () => void;
  private readonly error: (error: unknown) => void;
  constructor(
    load: (tile: Tile, signal: AbortSignal) => Promise<T>,
    changed: () => void,
    error: (error: unknown) => void,
  ) {
    this.load = load;
    this.changed = changed;
    this.error = error;
  }
  update(tiles: Tile[]): void {
    // 平移后取消离开视口的下载，把名额留给当前可见图块。
    this.wanted = tiles;
    const keys = new Set(tiles.map((tile) => tile.key));
    for (const [key, controller] of this.active) {
      if (!keys.has(key)) controller.abort();
    }
    this.pump();
  }
  reset(): void {
    // 切换存档时释放图片，并让迟到响应无法混入新地图。
    this.epoch++;
    for (const controller of this.active.values()) controller.abort();
    for (const image of this.images.values()) image.close();
    this.active.clear();
    this.images.clear();
    this.failed.clear();
    this.wanted = [];
  }
  private pump(): void {
    // 下载完成立即补位，取消的请求归还名额前仍计入并发数。
    for (const tile of this.wanted) {
      if (this.active.size >= 2) break;
      if (
        this.images.has(tile.key)
        || this.active.has(tile.key)
        || this.failed.has(tile.key)
      ) continue;
      const controller = new AbortController();
      const epoch = this.epoch;
      this.active.set(tile.key, controller);
      void this.load(tile, controller.signal).then((image) => {
        if (controller.signal.aborted || epoch !== this.epoch) {
          image.close();
          return;
        }
        this.images.set(tile.key, image);
        const wanted = new Set(this.wanted.map((entry) => entry.key));
        for (const [key, previous] of this.images) {
          if (this.images.size <= 256) break;
          if (!wanted.has(key)) {
            previous.close();
            this.images.delete(key);
          }
        }
        this.changed();
      }).catch((error: unknown) => {
        if (!controller.signal.aborted && epoch === this.epoch) {
          this.failed.add(tile.key);
          this.error(error);
        }
      }).finally(() => {
        if (this.active.get(tile.key) === controller) this.active.delete(tile.key);
        this.pump();
      });
    }
  }
}

export class Minimap {
  // 一个 dialog 在角落与模态覆盖层之间切换，浏览器负责隔离展开后的三维输入。
  private readonly panel = document.getElementById("minimap") as HTMLDialogElement;
  private readonly canvas = document.getElementById("minimap-canvas") as HTMLCanvasElement;
  private readonly context = this.canvas.getContext("2d")!;
  private readonly readout = document.getElementById("minimap-selection") as HTMLOutputElement;
  private readonly status = document.getElementById("minimap-status") as HTMLOutputElement;
  private readonly scaleLabel = document.getElementById("minimap-scale") as HTMLOutputElement;
  private readonly go = document.getElementById("minimap-jump") as HTMLButtonElement;
  private readonly tiles: MapTiles<ImageBitmap>;
  private readonly resize: ResizeObserver;
  private world?: WorldInfo;
  private index?: MapIndex;
  private center: Point = { x: 0, z: 0 };
  private scale = 1;
  private selection?: Selection;
  private indexRequest?: AbortController;
  private surfaceRequest?: AbortController;
  private pointer?: {
    id: number;
    x: number;
    y: number;
    center: Point;
    moved: boolean;
  };
  private dirty = true;
  private lastCamera = { x: 0, y: 80, z: 0, yaw: 0 };
  expanded = false;

  private readonly jump: (position: [number, number, number]) => void;
  private readonly modalChanged: () => void;
  private readonly focusScene: () => void;
  constructor(
    jump: (position: [number, number, number]) => void,
    modalChanged: () => void,
    focusScene: () => void,
  ) {
    this.jump = jump;
    this.modalChanged = modalChanged;
    this.focusScene = focusScene;
    this.tiles = new MapTiles(
      async (tile, signal) => {
        const query = new URLSearchParams({
          world: String(this.world!.revision),
          kind: tile.kind,
          x: String(tile.x),
          z: String(tile.z),
        });
        const response = await fetch(`/api/minimap/tile?${query}`, {
          signal,
          cache: "no-store",
        });
        if (!response.ok) throw new Error((await response.text()).trim());
        return createImageBitmap(await response.blob());
      },
      () => { this.dirty = true; },
      (error) => this.showError(error),
    );
    this.resize = new ResizeObserver(() => { this.dirty = true; });
    this.resize.observe(this.canvas);
    this.panel.addEventListener("cancel", (event) => {
      event.preventDefault();
      this.collapse();
    });
    document.getElementById("minimap-close")!.addEventListener("click", () => this.collapse());
    document.getElementById("minimap-retry")!.addEventListener("click", () => {
      this.tiles.failed.clear();
      this.status.value = "";
      if (!this.index) this.loadIndex();
      this.dirty = true;
    });
    this.go.addEventListener("click", () => {
      const point = this.selection!;
      this.collapse();
      this.jump([
        point.x,
        point.y == null ? this.lastCamera.y : point.y + 2,
        point.z,
      ]);
    });
    this.canvas.addEventListener("keydown", (event) => {
      if (!this.expanded && (event.key === "Enter" || event.key === " ")) {
        event.preventDefault();
        this.expand();
      }
    });
    this.canvas.addEventListener("pointerdown", (event) => {
      if (event.button !== 0) return;
      this.pointer = {
        id: event.pointerId,
        x: event.clientX,
        y: event.clientY,
        center: { ...this.center },
        moved: false,
      };
      this.canvas.setPointerCapture(event.pointerId);
    });
    this.canvas.addEventListener("pointermove", (event) => {
      const pointer = this.pointer;
      if (!pointer || pointer.id !== event.pointerId || !this.expanded) return;
      const dx = event.clientX - pointer.x;
      const dy = event.clientY - pointer.y;
      pointer.moved ||= Math.hypot(dx, dy) > 4;
      if (!pointer.moved) return;
      this.center = {
        x: pointer.center.x - dx * this.scale,
        z: pointer.center.z - dy * this.scale,
      };
      this.dirty = true;
    });
    this.canvas.addEventListener("pointerup", (event) => {
      const pointer = this.pointer;
      this.pointer = undefined;
      if (!pointer || pointer.id !== event.pointerId) return;
      if (this.canvas.hasPointerCapture(event.pointerId)) this.canvas.releasePointerCapture(event.pointerId);
      if (pointer.moved) return;
      if (!this.expanded) this.expand();
      else {
        const rect = this.canvas.getBoundingClientRect();
        this.select(mapPoint(
          this.center,
          this.scale,
          rect.width,
          rect.height,
          event.clientX - rect.left,
          event.clientY - rect.top,
        ));
      }
    });
    this.canvas.addEventListener("pointercancel", () => { this.pointer = undefined; });
    this.canvas.addEventListener("wheel", (event) => {
      event.preventDefault();
      const rect = this.canvas.getBoundingClientRect();
      const x = event.clientX - rect.left;
      const y = event.clientY - rect.top;
      const anchor = mapPoint(this.center, this.scale, rect.width, rect.height, x, y);
      const factor = Math.exp(Math.max(-1, Math.min(1, event.deltaY * 0.002)));
      this.scale = Math.max(0.125, Math.min(128, this.scale * factor));
      if (this.expanded) {
        this.center = {
          x: anchor.x - (x - rect.width / 2) * this.scale,
          z: anchor.z - (y - rect.height / 2) * this.scale,
        };
      }
      this.dirty = true;
    }, { passive: false });
  }

  setWorld(world: WorldInfo): void {
    // 清空所有代次相关状态，旧索引和地表响应即使迟到也不能更新新地图。
    if (this.expanded) this.collapse();
    this.world = world;
    this.indexRequest?.abort();
    this.surfaceRequest?.abort();
    this.tiles.reset();
    this.index = undefined;
    this.selection = undefined;
    this.pointer = undefined;
    this.scale = 1;
    this.center = { x: world.position[0], z: world.position[2] };
    this.status.value = "";
    this.readout.value = "点选地图查看坐标";
    this.go.disabled = true;
    this.panel.close();
    document.body.classList.toggle("has-minimap", Boolean(world.minimap));
    if (world.minimap) {
      this.panel.show();
      this.loadIndex();
    }
    this.dirty = true;
  }

  private loadIndex(): void {
    // 索引后台加载，附近图块先直接取 section，大世界无需等全库扫描。
    this.indexRequest?.abort();
    const controller = this.indexRequest = new AbortController();
    this.status.value = "正在读取地图索引…";
    void fetch(`/api/minimap/index?world=${this.world!.revision}`, {
      signal: controller.signal,
      cache: "no-store",
    })
      .then(async (response) => {
        if (!response.ok) throw new Error((await response.text()).trim());
        const index: MapIndex = await response.json();
        if (controller.signal.aborted) return;
        this.index = index;
        this.status.value = "";
        this.dirty = true;
      }).catch((error: unknown) => {
        if (!controller.signal.aborted) this.showError(error);
      });
  }

  private expand(): void {
    // showModal 令场景与其他面板失去交互，先清掉已经按住的三维输入。
    this.expanded = true;
    this.modalChanged();
    this.panel.close();
    this.panel.dataset.expanded = "true";
    this.canvas.setAttribute("aria-label", "俯视地图，拖动平移，滚轮缩放，点击选点");
    this.panel.showModal();
    this.canvas.focus();
    this.dirty = true;
  }
  private collapse(): void {
    // 恢复角落地图和场景焦点，让三维输入只在模态地图关闭后继续。
    this.expanded = false;
    this.pointer = undefined;
    this.panel.close();
    this.panel.dataset.expanded = "false";
    this.canvas.setAttribute("aria-label", "小地图，滚轮缩放，点击展开");
    this.panel.show();
    this.modalChanged();
    this.focusScene();
    this.dirty = true;
  }
  private select(point: Point): void {
    // 地表读取完成前禁用跳转，避免把尚未取得高度误认为没有地表。
    this.surfaceRequest?.abort();
    const controller = this.surfaceRequest = new AbortController();
    const selection: Selection = {
      x: Math.floor(point.x),
      z: Math.floor(point.z),
      pending: true,
    };
    this.selection = selection;
    this.go.disabled = true;
    this.readout.value = `X ${selection.x} · Z ${selection.z} · 读取地表…`;
    this.dirty = true;
    const query = new URLSearchParams({
      world: String(this.world!.revision),
      x: String(selection.x),
      z: String(selection.z),
    });
    void fetch(`/api/minimap/surface?${query}`, {
      signal: controller.signal,
      cache: "no-store",
    })
      .then(async (response) => {
        if (!response.ok) throw new Error((await response.text()).trim());
        const surface: { y: number | null } = await response.json();
        if (controller.signal.aborted) return;
        selection.y = surface.y;
        selection.pending = false;
        const height = surface.y === null ? "无地表，保留相机高度" : `Y ${surface.y}`;
        this.readout.value = `X ${selection.x} · Z ${selection.z} · ${height}`;
        this.go.disabled = false;
      }).catch((error: unknown) => {
        if (!controller.signal.aborted) {
          this.readout.value = `X ${selection.x} · Z ${selection.z} · 地表读取失败，请重新选点`;
          this.showError(error);
        }
      });
  }
  private showError(error: unknown): void {
    // 错误留在地图内，用户可以重试而不打断三维浏览。
    const message = error instanceof Error ? error.message : String(error);
    this.status.value = `地图加载失败：${message}。可重试。`;
  }

  update(x: number, y: number, z: number, yaw: number): void {
    // 主循环只在相机或地图变化时重画，收起时中心始终跟随相机。
    if (!this.panel.open) return;
    const moved = x !== this.lastCamera.x
      || z !== this.lastCamera.z
      || yaw !== this.lastCamera.yaw;
    this.lastCamera = { x, y, z, yaw };
    if (!this.expanded) this.center = { x, z };
    if (!this.dirty && !moved) return;
    this.dirty = false;
    const width = this.canvas.clientWidth;
    const height = this.canvas.clientHeight;
    const ratio = Math.min(devicePixelRatio, 2);
    this.canvas.width = Math.round(width * ratio);
    this.canvas.height = Math.round(height * ratio);
    const ctx = this.context;
    ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
    ctx.imageSmoothingEnabled = false;
    ctx.fillStyle = "#0c1018";
    ctx.fillRect(0, 0, width, height);
    const tiles = visibleTiles(this.center, this.scale, width, height, this.index);
    this.tiles.update(tiles);
    const screen = (point: Point): [number, number] => [
      width / 2 + (point.x - this.center.x) / this.scale,
      height / 2 + (point.z - this.center.z) / this.scale,
    ];
    for (const tile of tiles) {
      const size = tile.kind === "detail" ? 128 : 2048;
      const [startX, startY] = screen({ x: tile.x * size, z: tile.z * size });
      const [endX, endY] = screen({ x: (tile.x + 1) * size, z: (tile.z + 1) * size });
      // 相邻图块共用取整后的边界，避免小数像素上的透明混合露出背景。
      const left = Math.round(startX * ratio) / ratio;
      const top = Math.round(startY * ratio) / ratio;
      const right = Math.round(endX * ratio) / ratio;
      const bottom = Math.round(endY * ratio) / ratio;
      const image = this.tiles.images.get(tile.key);
      if (image) {
        ctx.drawImage(image, left, top, right - left, bottom - top);
      } else {
        ctx.strokeStyle = "rgba(255,201,138,0.12)";
        ctx.strokeRect(left, top, right - left, bottom - top);
      }
    }
    if (this.selection && this.expanded) {
      const [sx, sy] = screen(this.selection);
      ctx.strokeStyle = "#ffc98a";
      ctx.lineWidth = 1.5;
      ctx.beginPath();
      ctx.arc(sx, sy, 8, 0, Math.PI * 2);
      ctx.moveTo(sx - 13, sy);
      ctx.lineTo(sx + 13, sy);
      ctx.moveTo(sx, sy - 13);
      ctx.lineTo(sx, sy + 13);
      ctx.stroke();
    }
    const [cx, cy] = screen({ x, z });
    ctx.save();
    ctx.translate(cx, cy);
    ctx.rotate(-yaw);
    ctx.shadowColor = "#000";
    ctx.shadowBlur = 5;
    ctx.fillStyle = "#f7e8d2";
    ctx.strokeStyle = "#2b2119";
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(0, -10);
    ctx.lineTo(6, 7);
    ctx.lineTo(0, 4);
    ctx.lineTo(-6, 7);
    ctx.closePath();
    ctx.stroke();
    ctx.fill();
    ctx.restore();
    ctx.fillStyle = "#f7e8d2";
    ctx.font = "11px ui-monospace, monospace";
    ctx.fillText("N ↑", 12, 21);
    const scale = this.scale.toFixed(this.scale < 1 ? 2 : 1);
    const mode = tiles[0]?.kind === "overview" ? " · 区块分布" : "";
    this.scaleLabel.value = `${scale} 方块 / px${mode}`;
  }

  dispose(): void {
    // 页面退出时取消下载并释放位图，避免资源随后台回调继续存活。
    this.indexRequest?.abort();
    this.surfaceRequest?.abort();
    this.tiles.reset();
    this.resize.disconnect();
    this.panel.close();
  }
}
