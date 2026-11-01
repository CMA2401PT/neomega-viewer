/** Resident geometry is identified by world coordinates, never camera settings. */
export interface SectionKey {
  x: number;
  z: number;
  y: number;
}
export function keyID(key: SectionKey): string {
  return `${key.x},${key.z},${key.y}`;
}
export const MAX_RADIUS = 30;
export const verticalRadius = (radius: number): number =>
  Math.min(radius + 1, 12);

export function nearby(
  x: number,
  z: number,
  y: number,
  radius: number,
): SectionKey[] {
  const result: SectionKey[] = [];
  const height = verticalRadius(radius);
  for (let dx = -radius; dx <= radius; dx++) {
    for (let dz = -radius; dz <= radius; dz++) {
      if (dx * dx + dz * dz > radius * radius) continue;
      const cx = x + dx,
        cz = z + dz;
      if (cx < -1875000 || cx >= 1875000 || cz < -1875000 || cz >= 1875000)
        continue;
      for (
        let sy = Math.max(-128, y - height);
        sy <= Math.min(127, y + height);
        sy++
      ) {
        result.push({ x: cx, z: cz, y: sy });
      }
    }
  }
  return result.sort(
    (a, b) =>
      (a.x - x) ** 2 + (a.z - z) ** 2 - (b.x - x) ** 2 - (b.z - z) ** 2 ||
      Math.abs(a.y - y) - Math.abs(b.y - y),
  );
}
export interface Resident {
  bytes: number;
  dispose(): void;
}
export interface StreamStats {
  loaded: number;
  wanted: number;
  pending: number;
  bytes: number;
  error: string;
}
interface Job {
  keys: SectionKey[];
  controller: AbortController;
}
type Load<T> = (keys: SectionKey[], signal: AbortSignal) => Promise<T[]>;

/** Keep overlapping sections alive. Batch consecutive missing sections from one
 * column into one request; a partially relevant job is allowed to finish. */
export class ChunkStream<T extends Resident> {
  readonly loaded = new Map<string, T>();
  private desired = new Map<string, SectionKey>();
  private rank = new Map<string, number>();
  private pending = new Map<string, Job>();
  private jobs = new Set<Job>();
  private skipped = new Set<string>();
  private bytes = 0;
  private signature = "";
  private stopped = false;
  private error = "";
  private readonly load: Load<T>;
  private readonly attach: (key: SectionKey, value: T) => void;
  private readonly changed: (stats: StreamStats) => void;
  private readonly concurrency: number;

  constructor(
    load: Load<T>,
    attach: (key: SectionKey, value: T) => void,
    changed: (stats: StreamStats) => void,
    concurrency = 2,
  ) {
    this.load = load;
    this.attach = attach;
    this.changed = changed;
    this.concurrency = concurrency;
  }

  update(x: number, z: number, y: number, radius: number): void {
    const signature = `${x},${z},${y},${radius}`;
    if (this.stopped || signature === this.signature) return;
    this.signature = signature;
    const keys = nearby(x, z, y, radius);
    this.desired = new Map(keys.map((key) => [keyID(key), key]));
    this.rank = new Map(keys.map((key, i) => [keyID(key), i]));
    this.skipped.clear();
    this.error = "";
    for (const [id, value] of this.loaded)
      if (!this.desired.has(id)) this.remove(id, value);
    for (const job of this.jobs) {
      if (job.keys.every((key) => !this.desired.has(keyID(key))))
        job.controller.abort();
    }
    this.pump();
  }

  private remove(id: string, value: T): void {
    this.loaded.delete(id);
    this.bytes -= value.bytes;
    value.dispose();
  }
  private available(key: SectionKey): boolean {
    const id = keyID(key);
    return (
      this.desired.has(id) &&
      !this.loaded.has(id) &&
      !this.pending.has(id) &&
      !this.skipped.has(id)
    );
  }
  private batch(key: SectionKey): SectionKey[] {
    const keys = [key];
    for (let y = key.y - 1; keys.length < 15; y--) {
      const next = { ...key, y };
      if (!this.available(next)) break;
      keys.unshift(next);
    }
    for (let y = key.y + 1; keys.length < 15; y++) {
      const next = { ...key, y };
      if (!this.available(next)) break;
      keys.push(next);
    }
    return keys;
  }
  private accept(key: SectionKey, value: T, job: Job): void {
    const id = keyID(key);
    if (
      this.stopped ||
      job.controller.signal.aborted ||
      !this.desired.has(id)
    ) {
      value.dispose();
      return;
    }
    this.loaded.set(id, value);
    this.bytes += value.bytes;
    this.attach(key, value);
  }
  private pump(): void {
    if (this.stopped) return;
    for (const key of this.desired.values()) {
      if (this.jobs.size >= this.concurrency) break;
      if (!this.available(key)) continue;
      const job: Job = {
        keys: this.batch(key),
        controller: new AbortController(),
      };
      this.jobs.add(job);
      for (const key of job.keys) this.pending.set(keyID(key), job);
      void this.load(job.keys, job.controller.signal)
        .then((values) => {
          // Install the nearest sections first even though the response is height ordered.
          const entries = job.keys.map((key, i) => ({ key, value: values[i] }));
          entries.sort(
            (a, b) =>
              (this.rank.get(keyID(a.key)) ?? Infinity) -
              (this.rank.get(keyID(b.key)) ?? Infinity),
          );
          for (const { key, value } of entries) this.accept(key, value, job);
        })
        .catch((error: unknown) => {
          if (!job.controller.signal.aborted && !this.stopped) {
            for (const key of job.keys) this.skipped.add(keyID(key));
            this.error = error instanceof Error ? error.message : String(error);
          }
        })
        .finally(() => {
          for (const key of job.keys) this.pending.delete(keyID(key));
          this.jobs.delete(job);
          this.pump();
        });
    }
    this.changed({
      loaded: this.loaded.size,
      wanted: this.desired.size,
      pending: this.jobs.size,
      bytes: this.bytes,
      error: this.error,
    });
  }
  dispose(): void {
    this.stopped = true;
    for (const job of this.jobs) job.controller.abort();
    for (const [id, value] of this.loaded) this.remove(id, value);
  }
}
