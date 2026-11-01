import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { ChunkStream, nearby, keyID, MAX_RADIUS } from "../src/stream.ts";
import { decodeColumn } from "../src/packet.ts";

const tick = () => new Promise((resolve) => setImmediate(resolve));

// Compare object identity as well as requests: a cache hit must retain GPU objects.
test("horizontal movement, vertical movement and radius growth retain overlapping sections", async () => {
  const requests = [],
    disposed = new Set();
  const stream = new ChunkStream(
    async (keys) => {
      requests.push(...keys.map(keyID));
      return keys.map((key) => ({
        bytes: 1,
        dispose() {
          disposed.add(keyID(key));
        },
      }));
    },
    () => {},
    () => {},
  );
  stream.update(0, 0, 0, 1);
  await tick();
  for (const view of [
    [1, 0, 0, 1],
    [1, 0, 1, 1],
    [1, 0, 1, 2],
  ]) {
    const before = new Map(stream.loaded);
    requests.length = 0;
    disposed.clear();
    stream.update(...view);
    await tick();
    const wanted = new Set(nearby(...view).map(keyID));
    for (const [id, value] of before) {
      if (wanted.has(id)) {
        assert.equal(stream.loaded.get(id), value, `${id} was replaced`);
        assert(!requests.includes(id), `${id} was downloaded twice`);
        assert(!disposed.has(id), `${id} was disposed`);
      } else assert(disposed.has(id));
    }
    assert.deepEqual(
      new Set(requests),
      new Set([...wanted].filter((id) => !before.has(id))),
    );
  }
  stream.dispose();
});

test("teleport cancels obsolete jobs and releases late results with two-request concurrency", async () => {
  const pending = [],
    attached = [];
  let disposed = 0;
  const stream = new ChunkStream(
    (keys, signal) =>
      new Promise((resolve) => pending.push({ keys, signal, resolve })),
    (key) => attached.push(key),
    () => {},
  );
  stream.update(0, 0, 0, 1);
  assert.equal(pending.length, 2);
  stream.update(-100, 200, -4, 1);
  assert(pending.every((job) => job.signal.aborted));
  const settle = (job) =>
    job.resolve(job.keys.map(() => ({ bytes: 10, dispose: () => disposed++ })));
  pending.slice(0, 2).forEach(settle);
  await tick();
  assert.equal(disposed, 10);
  assert.equal(attached.length, 0);
  assert.equal(pending.length, 4);
  stream.dispose();
  pending.slice(2).forEach(settle);
  await tick();
  assert.equal(disposed, 20);
  assert.equal(pending.length, 4);
});

test("a partly relevant pending batch survives a height change", async () => {
  const pending = [];
  const stream = new ChunkStream(
    (keys, signal) =>
      new Promise((resolve) => pending.push({ keys, signal, resolve })),
    () => {},
    () => {},
  );
  stream.update(0, 0, 0, 1);
  stream.update(0, 0, 1, 1);
  assert(pending.every((job) => !job.signal.aborted));
  for (const job of pending.slice())
    job.resolve(job.keys.map(() => ({ bytes: 1, dispose() {} })));
  await tick();
  assert(stream.loaded.has("0,0,0"));
  assert(!stream.loaded.has("0,0,-2"));
  stream.dispose();
});

test("all requested sections remain resident beyond the former 512 MiB budget", async () => {
  let stats;
  const sectionBytes = 32 * 1024 * 1024;
  const stream = new ChunkStream(
    async (keys) => keys.map(() => ({ bytes: sectionBytes, dispose() {} })),
    () => {},
    (value) => { stats = value; },
  );
  stream.update(0, 0, 0, 1);
  await tick();
  const wanted = nearby(0, 0, 0, 1);
  assert.equal(stream.loaded.size, wanted.length);
  assert.equal(stats.pending, 0);
  assert.equal(stats.bytes, wanted.length * sectionBytes);
  assert(stats.bytes > 512 * 1024 * 1024);
  assert.deepEqual(new Set(stream.loaded.keys()), new Set(wanted.map(keyID)));
  stream.dispose();
  assert.equal(stream.loaded.size, 0);
});

test("radius 30 requests the full circular range", () => {
  const keys = nearby(0, 0, 0, MAX_RADIUS);
  assert.equal(new Set(keys.map((key) => `${key.x},${key.z}`)).size, 2821);
  assert.equal(keys.length, 2821 * 25);
});

test("world packet preserves section ownership and negative Y", () => {
  const frame = readFileSync(
    new URL("./fixtures/block_entities.nvframe", import.meta.url),
  );
  const packet = new ArrayBuffer(28 + frame.length);
  const view = new DataView(packet);
  new Uint8Array(packet).set(new TextEncoder().encode("NVWC"));
  view.setUint32(4, 2, true);
  view.setUint32(8, 1, true);
  view.setUint32(12, 7, true);
  view.setInt32(16, -4, true);
  view.setUint32(20, frame.length, true);
  view.setUint32(24, 7, true);
  new Uint8Array(packet, 28).set(frame);
  const result = decodeColumn(packet);
  assert.equal(result.sections[0].unknown, 7);
  assert.equal(result.sections[0].y, -4);
  assert.equal(result.sections[0].bytes, frame.length);
  assert.equal(result.sections[0].frame.actors.length, 6);
  assert.throws(() => decodeColumn(packet.slice(0, -1)));
  view.setUint32(20, 0xffffffff, true);
  assert.throws(() => decodeColumn(packet));
});
