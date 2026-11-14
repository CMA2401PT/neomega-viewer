import type { SectionPos } from "@neomega/viewer/render";

/** Resident geometry is identified by world coordinates, never camera settings. */
export type SectionKey = SectionPos;
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
interface Job<Payload, Decoded> {
  keys: SectionKey[];
  controller: AbortController;
  state:
    | { kind: "fetching" }
    | { kind: "ready"; payload: Payload }
    | { kind: "processing"; data: Decoded; remaining: SectionKey[] };
}
interface StreamOptions<T, Payload, Decoded> {
  load(keys: SectionKey[], signal: AbortSignal): Promise<Payload>;
  decode(payload: Payload): Decoded;
  prepare(key: SectionKey, data: Decoded): T;
  attach(key: SectionKey, value: T): void;
  changed(stats: StreamStats): void;
  concurrency?: number;
}

/** Network slots end when response bytes arrive. Main-thread work is drained by
 * processFrame, with at most two columns per network slot resident in the entire
 * pipeline. This bounds prefetch without limiting resident world geometry. */
export class ChunkStream<T extends Resident, Payload, Decoded> {
  readonly loaded = new Map<string, T>();
  private desired = new Map<string, SectionKey>();
  private rank = new Map<string, number>();
  private pending = new Map<string, Job<Payload, Decoded>>();
  private jobs = new Set<Job<Payload, Decoded>>();
  private ready = new Set<Job<Payload, Decoded>>();
  private active?: Job<Payload, Decoded>;
  private fetching = 0;
  private skipped = new Set<string>();
  private bytes = 0;
  private signature = "";
  private stopped = false;
  private error = "";
  private readonly options: StreamOptions<T, Payload, Decoded>;
  private readonly concurrency: number;

  constructor(options: StreamOptions<T, Payload, Decoded>) {
    this.options = options;
    this.concurrency = options.concurrency ?? 4;
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
      if (job.keys.every((key) => !this.desired.has(keyID(key)))) {
        job.controller.abort();
        // Fetching jobs keep their slot until the abort settles. Queued bytes
        // and decoded data can be dropped immediately, without making meshes.
        if (job.state.kind !== "fetching") this.finish(job);
      } else if (job.state.kind === "processing") {
        job.state.remaining = this.prioritize(job.state.remaining);
      }
    }
    this.pump();
    this.publish();
  }

  /** One decode or section preparation is indivisible. Stop before starting the
   * next operation once the frame budget has been used. Attach each section as
   * soon as it is ready, so large responses do not delay their nearest geometry. */
  processFrame(budgetMS = 4): void {
    if (this.stopped || (!this.active && !this.ready.size)) return;
    const deadline = performance.now() + budgetMS;
    do {
      const job = this.active ?? this.nearestReady();
      if (!job) break;
      this.active = job;
      try {
        if (job.state.kind === "ready") {
          const data = this.options.decode(job.state.payload);
          job.state = { kind: "processing", data, remaining: this.prioritize(job.keys) };
          this.ready.delete(job);
        } else if (job.state.kind === "processing") {
          const key = job.state.remaining.shift();
          if (key) {
            const value = this.options.prepare(key, job.state.data);
            this.loaded.set(keyID(key), value);
            this.bytes += value.bytes;
            this.options.attach(key, value);
          }
        }
        if (job.state.kind === "processing" && !job.state.remaining.length) this.finish(job);
      } catch (error: unknown) {
        this.fail(job, error);
        this.finish(job);
      }
    } while (performance.now() < deadline);
    this.pump();
    this.publish();
  }

  private prioritize(keys: SectionKey[]): SectionKey[] {
    return keys.filter((key) => this.desired.has(keyID(key)))
      .sort((a, b) => this.rank.get(keyID(a))! - this.rank.get(keyID(b))!);
  }

  private nearestReady(): Job<Payload, Decoded> | undefined {
    let nearest: Job<Payload, Decoded> | undefined;
    let best = Infinity;
    for (const job of this.ready) {
      for (const key of job.keys) {
        const rank = this.rank.get(keyID(key)) ?? Infinity;
        if (rank < best) {
          nearest = job;
          best = rank;
        }
      }
    }
    return nearest;
  }

  private remove(id: string, value: T): void {
    this.loaded.delete(id);
    this.bytes -= value.bytes;
    value.dispose();
  }
  private available(key: SectionKey): boolean {
    const id = keyID(key);
    return this.desired.has(id) && !this.loaded.has(id)
      && !this.pending.has(id) && !this.skipped.has(id);
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

  private finish(job: Job<Payload, Decoded>): void {
    for (const key of job.keys) this.pending.delete(keyID(key));
    this.jobs.delete(job);
    this.ready.delete(job);
    if (this.active === job) this.active = undefined;
  }

  private fail(job: Job<Payload, Decoded>, error: unknown): void {
    if (job.controller.signal.aborted || this.stopped) return;
    for (const key of job.keys) this.skipped.add(keyID(key));
    this.error = error instanceof Error ? error.message : String(error);
  }

  private pump(): void {
    if (this.stopped) return;
    for (const key of this.desired.values()) {
      if (this.fetching >= this.concurrency || this.jobs.size >= this.concurrency * 2) break;
      if (!this.available(key)) continue;
      const job: Job<Payload, Decoded> = {
        keys: this.batch(key),
        controller: new AbortController(),
        state: { kind: "fetching" },
      };
      this.jobs.add(job);
      this.fetching++;
      for (const key of job.keys) this.pending.set(keyID(key), job);
      void this.options.load(job.keys, job.controller.signal)
        .then((payload) => {
          if (this.stopped || job.controller.signal.aborted) {
            this.finish(job);
            return;
          }
          job.state = { kind: "ready", payload };
          this.ready.add(job);
        })
        .catch((error: unknown) => {
          this.fail(job, error);
          this.finish(job);
        })
        .finally(() => {
          this.fetching--;
          this.pump();
          if (!this.stopped) this.publish();
        });
    }
  }

  private publish(): void {
    this.options.changed({
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
    this.jobs.clear();
    this.ready.clear();
    this.pending.clear();
    this.active = undefined;
    for (const [id, value] of this.loaded) this.remove(id, value);
  }
}
