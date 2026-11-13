import { decodeVolume, type Volume } from "../../../../../../neomega-viewer-core/viewer/brickmap/web/volume.ts";

export const REGION_SIZE = 128;
export const WINDOW_REGIONS = 12;
export const WINDOW_SIZE = REGION_SIZE * WINDOW_REGIONS;

export interface RegionPosition { x: number; z: number }
export interface RegionRequest extends RegionPosition {
  key: string;
  state: "queued" | "loading" | "ready" | "resident" | "full" | "failed";
  controller?: AbortController;
  volume?: Volume;
}
export const regionKey = ({ x, z }: RegionPosition): string => `${x}/${z}`;

export function windowOrigin(x: number, z: number): RegionPosition {
  // 按水平位置选择最近的区域中心，与视线朝向和命中的方块无关。
  return {
    x: Math.floor(x / REGION_SIZE + 0.5) - WINDOW_REGIONS / 2,
    z: Math.floor(z / REGION_SIZE + 0.5) - WINDOW_REGIONS / 2,
  };
}

export class RegionDownloads {
  readonly entries = new Map<string, RegionRequest>();
  private readonly active = new Set<RegionRequest>();
  private revision = 0;
  private readonly load: (revision: number, position: RegionPosition, signal: AbortSignal) => Promise<Volume>;
  private readonly error: (error: unknown) => void;

  constructor(
    error: (error: unknown) => void,
    load = async (revision: number, position: RegionPosition, signal: AbortSignal): Promise<Volume> => {
      const query = new URLSearchParams({ world: String(revision), x: String(position.x), z: String(position.z) });
      const response = await fetch(`/api/minimap/region?${query}`, { signal, cache: "no-store" });
      if (!response.ok) throw new Error((await response.text()).trim());
      return decodeVolume(await response.arrayBuffer());
    },
  ) {
    // 下载完成只交付 CPU 数据，显存分配和目录发布留给主帧循环。
    this.error = error;
    this.load = load;
  }

  reset(revision: number): void {
    // 已取消的请求仍占名额直到真正退出，旧结果通过区域实例身份丢弃。
    this.revision = revision;
    for (const request of this.active) request.controller!.abort();
    this.entries.clear();
  }

  update(positions: readonly RegionPosition[]): void {
    // 同一窗口内的区域实例保持不变，因此窗口平移不会废弃仍有用的迟到结果。
    const wanted = new Set(positions.map(regionKey));
    for (const [key, request] of this.entries) if (!wanted.has(key)) {
      request.controller?.abort();
      this.entries.delete(key);
    }
    const ordered = positions.map(position => {
      const key = regionKey(position);
      let request = this.entries.get(key);
      if (!request) {
        request = { ...position, key, state: "queued" };
        this.entries.set(key, request);
      }
      return request;
    });
    for (const request of ordered) {
      // 待上传响应也占名额，下载速度超过上传速度时最多保留两份区域字节。
      if (this.active.size + [...this.entries.values()].filter(entry => entry.state === "ready").length >= 2) break;
      if (request.state !== "queued") continue;
      request.state = "loading";
      const controller = request.controller = new AbortController();
      this.active.add(request);
      void this.load(this.revision, request, controller.signal).then(volume => {
        if (controller.signal.aborted || this.entries.get(request.key) !== request) return;
        request.volume = volume;
        request.state = "ready";
      }).catch((error: unknown) => {
        if (controller.signal.aborted || this.entries.get(request.key) !== request) return;
        request.state = "failed";
        this.error(error);
      }).finally(() => this.active.delete(request));
    }
  }

  retry(): void {
    // 明确的重试只重排失败与容量不足的区域，已安装的区域继续复用。
    for (const request of this.entries.values()) {
      if (request.state === "failed" || request.state === "full") request.state = "queued";
    }
  }
}
