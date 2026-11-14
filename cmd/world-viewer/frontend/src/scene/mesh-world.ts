import * as THREE from "three";
import { SectionScene, type EffectRegistry, type SectionObject, type SurfaceStyle } from "@neomega/viewer/render";
import { ChunkStream, type SectionKey, type StreamStats } from "./stream.ts";
import { decodeColumn, type RenderIssue } from "../api/packet.ts";

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
interface MeshWorldOptions {
  scene: THREE.Scene;
  atlas: THREE.Texture;
  effects: EffectRegistry;
  style?: SurfaceStyle;
  revision: number;
  issues(issues: RenderIssue[], column: [number, number]): void;
  changed(stats: StreamStats): void;
}

export class MeshWorld {
  // 一个存档实例同时拥有下载队列和挂载的网格，切换时统一取消并释放。
  readonly scene: SectionScene;
  readonly stream: ChunkStream<LoadedSection, ColumnPayload, Map<number, ColumnSection>>;
  revision = 0;

  constructor(options: MeshWorldOptions) {
    const draw = this.scene = new SectionScene(
      options.scene,
      options.atlas,
      options.effects,
      16,
      options.style,
    );
    this.stream = new ChunkStream({
      load: (keys, signal) => fetchColumn(keys, signal, options.revision),
      decode: (payload: ColumnPayload) => {
        const data = decodeColumn(payload.buffer);
        options.issues(data.issues, payload.column);
        return new Map(data.sections.map((section) => [section.y, section]));
      },
      prepare: (key, sections) => {
        const section = sections.get(key.y);
        const object = draw.prepare(key, section?.frame);
        return {
          section: object,
          unknown: section?.unknown ?? 0,
          bytes: object.bytes,
          dispose: () => { object.dispose(); this.revision++; },
        };
      },
      attach: (_key, value) => { draw.add(value.section); this.revision++; },
      changed: options.changed,
    });
  }

  *sections(): Iterable<SectionKey> {
    // 空 section 也属于已加载范围，远景必须同样让出它们。
    for (const value of this.stream.loaded.values()) yield value.section.key;
  }

  get unknown(): number {
    let count = 0;
    for (const value of this.stream.loaded.values()) count += value.unknown;
    return count;
  }

  dispose(): void {
    this.stream.dispose();
    this.scene.dispose();
  }
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
