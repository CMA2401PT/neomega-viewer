import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { ChunkStream, nearby, keyID, MAX_RADIUS } from "../src/stream.ts";
import { decodeColumn } from "../src/packet.ts";

const tick = () => new Promise((resolve) => setImmediate(resolve));

const states = new WeakMap();
function makeStream(load, attach, changed, concurrency = 2) {
  const stream = new ChunkStream({
    load: async (keys, signal) => ({ keys, values: await load(keys, signal) }),
    decode: ({ keys, values }) => new Map(keys.map((key, i) => [keyID(key), values[i]])),
    prepare: (key, values) => values.get(keyID(key)),
    attach,
    changed: (stats) => { states.set(stream, stats); changed(stats); },
    concurrency,
  });
  return stream;
}
async function drain(stream) {
  while (states.get(stream)?.pending) {
    await tick();
    stream.processFrame(Infinity);
  }
}

// Compare object identity as well as requests: a cache hit must retain GPU objects.
test("horizontal movement, vertical movement and radius growth retain overlapping sections", async () => {
  const requests = [],
    disposed = new Set();
  const stream = makeStream(
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
  await drain(stream);
  for (const view of [
    [1, 0, 0, 1],
    [1, 0, 1, 1],
    [1, 0, 1, 2],
  ]) {
    const before = new Map(stream.loaded);
    requests.length = 0;
    disposed.clear();
    stream.update(...view);
    await drain(stream);
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

test("teleport cancels obsolete downloads and drops late payloads before mesh preparation", async () => {
  const pending = [],
    attached = [];
  let disposed = 0;
  const stream = makeStream(
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
  assert.equal(disposed, 0);
  assert.equal(attached.length, 0);
  assert.equal(pending.length, 4);
  stream.dispose();
  pending.slice(2).forEach(settle);
  await tick();
  assert.equal(disposed, 0);
  assert.equal(pending.length, 4);
});

test("a partly relevant pending batch survives a height change", async () => {
  const pending = [];
  const stream = makeStream(
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
  stream.processFrame(Infinity);
  assert(stream.loaded.has("0,0,0"));
  assert(!stream.loaded.has("0,0,-2"));
  stream.dispose();
});

test("all requested sections remain resident beyond the former 512 MiB budget", async () => {
  let stats;
  const sectionBytes = 32 * 1024 * 1024;
  const stream = makeStream(
    async (keys) => keys.map(() => ({ bytes: sectionBytes, dispose() {} })),
    () => {},
    (value) => { stats = value; },
  );
  stream.update(0, 0, 0, 1);
  await drain(stream);
  const wanted = nearby(0, 0, 0, 1);
  assert.equal(stream.loaded.size, wanted.length);
  assert.equal(stats.pending, 0);
  assert.equal(stats.bytes, wanted.length * sectionBytes);
  assert(stats.bytes > 512 * 1024 * 1024);
  assert.deepEqual(new Set(stream.loaded.keys()), new Set(wanted.map(keyID)));
  stream.dispose();
  assert.equal(stream.loaded.size, 0);
});

test("downloads refill independently while queued columns stay bounded and preparation stays on frame", async () => {
  const requests = [], attached = [];
  let decoded = 0, prepared = 0, stats;
  const stream = new ChunkStream({
    load: (keys, signal) => new Promise((resolve) => requests.push({ keys, signal, resolve })),
    decode: (payload) => { decoded++; return payload; },
    prepare: () => { prepared++; return { bytes: 1, dispose() {} }; },
    attach: (key) => attached.push(keyID(key)),
    changed: (value) => stats = value,
  });
  stream.update(0, 0, 0, 2);
  assert.equal(requests.length, 4);
  requests.slice().forEach((job) => job.resolve({}));
  await tick();
  assert.equal(requests.length, 8, "network slots must refill without preparing meshes");
  requests.slice(4).forEach((job) => job.resolve({}));
  await tick();
  assert.equal(requests.length, 8, "completed responses must apply backpressure");
  assert.equal(stats.pending, 8);
  assert.equal(decoded, 0);
  assert.equal(prepared, 0);
  stream.processFrame(0);
  assert.equal(decoded, 1);
  assert.equal(prepared, 0, "a decode that uses the budget must defer preparation");
  stream.processFrame(0);
  assert.equal(prepared, 1);
  assert.deepEqual(attached, ["0,0,0"], "nearest section attaches before the rest of its column");
  stream.processFrame(Infinity);
  assert.equal(requests.length, 12);
  stream.dispose();
  assert(requests.slice(8).every((job) => job.signal.aborted));
});

test("moving discards queued bytes and obsolete active sections without preparing them", async () => {
  const attached = [];
  let prepared = 0, disposed = 0;
  const stream = new ChunkStream({
    load: async () => ({}),
    decode: (value) => value,
    prepare: () => { prepared++; return { bytes: 1, dispose() { disposed++; } }; },
    attach: (key) => attached.push(keyID(key)),
    changed() {},
  });
  stream.update(0, 0, 0, 2);
  await tick();
  stream.processFrame(0); // Decode one column.
  stream.processFrame(0); // Attach its nearest section.
  assert.equal(prepared, 1);
  stream.update(100, -100, 0, 1);
  assert.equal(disposed, 1);
  attached.length = 0;
  await tick();
  stream.processFrame(Infinity);
  const wanted = new Set(nearby(100, -100, 0, 1).map(keyID));
  assert(attached.length > 0);
  assert(attached.every((key) => wanted.has(key)));
  assert.equal(prepared, attached.length + 1, "obsolete decoded sections were prepared");
  stream.dispose();
  await tick();
  assert.equal(disposed, prepared);
});

test("partly processed columns survive height changes and reload sections that become wanted again", async () => {
  let stats;
  const attached = [];
  const stream = new ChunkStream({
    load: async () => ({}),
    decode: (value) => value,
    prepare: () => ({ bytes: 1, dispose() {} }),
    attach: (key) => attached.push(keyID(key)),
    changed(value) { stats = value; },
  });
  stream.update(0, 0, 0, 1);
  await tick();
  stream.processFrame(0);
  stream.processFrame(0);
  const retained = stream.loaded.get("0,0,0");
  stream.update(0, 0, 1, 1); // Drop Y=-2 from the active column's remaining work.
  stream.update(0, 0, 0, 1); // It must become eligible again after that column finishes.
  while (stats.pending) {
    await tick();
    stream.processFrame(Infinity);
  }
  assert.equal(stream.loaded.get("0,0,0"), retained);
  assert.deepEqual(new Set(stream.loaded.keys()), new Set(nearby(0, 0, 0, 1).map(keyID)));
  assert.equal(attached.length, stream.loaded.size, "overlapping sections were attached twice");
  stream.dispose();
});

test("decode and preparation failures release capacity and keep other columns progressing", async () => {
  let stats;
  const stream = new ChunkStream({
    load: async (keys) => keys[0],
    decode: (key) => {
      if (key.x === 0 && key.z === 0) throw new Error("decode failed");
      return key;
    },
    prepare: (key) => {
      if (key.x === -1) throw new Error("prepare failed");
      return { bytes: 1, dispose() {} };
    },
    attach() {},
    changed(value) { stats = value; },
  });
  stream.update(0, 0, 0, 1);
  while (stats.pending) {
    await tick();
    stream.processFrame(Infinity);
  }
  assert.equal(stats.pending, 0);
  assert.equal(stream.loaded.size, 15);
  assert.match(stats.error, /failed/);
  stream.dispose();
});

test("radius 30 requests the full circular range", () => {
  const keys = nearby(0, 0, 0, MAX_RADIUS);
  assert.equal(new Set(keys.map((key) => `${key.x},${key.z}`)).size, 2821);
  assert.equal(keys.length, 2821 * 25);
});

test("world packet preserves section ownership and negative Y", () => {
  const frame = readFileSync(
    new URL("../../../../web/tests/fixtures/block_entities.nvframe", import.meta.url),
  );
  const issues = [{ scope: "block", origin: "render", count: 1, position: [-16, -64, 32], blockstate: "minecraft:grass []", reason: "missing_model" }];
  const diagnostics = new TextEncoder().encode(JSON.stringify(issues));
  const packet = new ArrayBuffer(32 + frame.length + diagnostics.length);
  const view = new DataView(packet);
  new Uint8Array(packet).set(new TextEncoder().encode("NVWC"));
  view.setUint32(4, 3, true);
  view.setUint32(8, 1, true);
  view.setUint32(12, 7, true);
  view.setUint32(16, diagnostics.length, true);
  view.setInt32(20, -4, true);
  view.setUint32(24, frame.length, true);
  view.setUint32(28, 7, true);
  new Uint8Array(packet, 32).set(frame);
  new Uint8Array(packet, 32 + frame.length).set(diagnostics);
  const result = decodeColumn(packet);
  assert.deepEqual(result.issues, issues);
  assert.equal(result.sections[0].unknown, 7);
  assert.equal(result.sections[0].y, -4);
  assert.equal(result.sections[0].bytes, frame.length);
  assert.equal(result.sections[0].frame.actors.length, 6);
  assert.throws(() => decodeColumn(packet.slice(0, -1)));
  view.setUint32(24, 0xffffffff, true);
  assert.throws(() => decodeColumn(packet));
});
