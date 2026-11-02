import type { Vector3 } from "three";
import {
  decodeFrame,
  type SectionObject,
  type SectionPos,
  type SectionScene,
} from "@neomega/viewer/render";

/** 围绕相机加载有限范围的地面子区块，离开范围后释放几何并取消请求。 */
export class SectionLoader {
  readonly loaded = new Map<string, SectionObject>();
  error = "";
  private wanted = new Map<string, SectionPos>();
  private pending = new Map<string, AbortController>();
  private center = "";
  private readonly world: SectionScene;
  private readonly radius: number;

  constructor(world: SectionScene, radius = 3) {
    this.world = world;
    this.radius = radius;
  }

  /** 跨过区块边界时重排近处优先的目标，每帧补足两个请求名额。 */
  update(position: Vector3): void {
    const x = Math.floor(position.x / 16), z = Math.floor(position.z / 16);
    const center = `${x},${z}`;
    if (center !== this.center) {
      this.center = center;
      const positions: SectionPos[] = [];
      for (let dx = -this.radius; dx <= this.radius; dx++) {
        for (let dz = -this.radius; dz <= this.radius; dz++) {
          positions.push({ x: x + dx, y: 0, z: z + dz });
        }
      }
      positions.sort((a, b) =>
        (a.x - x) ** 2 + (a.z - z) ** 2 - (b.x - x) ** 2 - (b.z - z) ** 2,
      );
      this.wanted = new Map(positions.map(pos => [`${pos.x},${pos.z}`, pos]));
      for (const [id, section] of this.loaded) {
        if (!this.wanted.has(id)) {
          section.dispose();
          this.loaded.delete(id);
        }
      }
      for (const [id, controller] of this.pending) {
        if (!this.wanted.has(id)) controller.abort();
      }
    }
    if (this.error) return;
    for (const [id, pos] of this.wanted) {
      if (this.pending.size >= 2) break;
      if (this.loaded.has(id) || this.pending.has(id)) continue;
      const controller = new AbortController();
      this.pending.set(id, controller);
      void this.load(id, pos, controller);
    }
  }

  private async load(id: string, pos: SectionPos, controller: AbortController): Promise<void> {
    try {
      const response = await fetch(
        `/api/section?x=${pos.x}&z=${pos.z}`,
        { signal: controller.signal },
      );
      if (!response.ok) throw new Error(await response.text());
      const buffer = await response.arrayBuffer();
      // 移动期间已经离开范围的响应不再解码、上传 GPU。
      controller.signal.throwIfAborted();
      const section = this.world.prepare(pos, decodeFrame(buffer));
      this.world.add(section);
      this.loaded.set(id, section);
    } catch (error: unknown) {
      if (!controller.signal.aborted) {
        this.error = String(error);
        console.error(error);
      }
    } finally {
      this.pending.delete(id);
    }
  }

  dispose(): void {
    for (const controller of this.pending.values()) controller.abort();
    for (const section of this.loaded.values()) section.dispose();
    this.loaded.clear();
  }
}
