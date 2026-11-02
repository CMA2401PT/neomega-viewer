import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { Vector3 } from "three";
import { SectionLoader } from "./stream.ts";

const fixture = readFileSync(new URL("../../web/tests/fixtures/block_entities.nvframe", import.meta.url));

test("跨区块保留重叠部分、释放离开的部分，驻留数量有界", async (t) => {
  const originalFetch = globalThis.fetch;
  t.after(() => { globalThis.fetch = originalFetch; });
  let requests = 0, disposed = 0, active = 0, maximumActive = 0;
  globalThis.fetch = async () => {
    requests++;
    active++;
    maximumActive = Math.max(maximumActive, active);
    await new Promise(setImmediate);
    active--;
    return new Response(fixture);
  };
  const world = {
    prepare(key, data) {
      assert.equal(data.effects.length, 1);
      return { key, dispose() { disposed++; } };
    },
    add() {},
  };
  const loader = new SectionLoader(world);
  const position = new Vector3(-0.1, 15, -0.1);
  for (let frame = 0; frame < 30; frame++) {
    loader.update(position);
    await new Promise(setImmediate);
  }
  assert.equal(loader.loaded.size, 49);
  assert.equal(requests, 49);
  assert.equal(maximumActive, 2);
  assert(loader.loaded.has("-1,-1"));
  position.x += 16;
  loader.update(position);
  assert.equal(loader.loaded.size, 42);
  assert.equal(disposed, 7);
  for (let frame = 0; frame < 10; frame++) {
    await new Promise(setImmediate);
    loader.update(position);
  }
  assert.equal(loader.loaded.size, 49);
  assert.equal(requests, 56);
  loader.dispose();
  assert.equal(disposed, 56);
  assert.equal(loader.loaded.size, 0);
});

test("离开范围后取消在途请求，即使迟到的响应成功也不挂载", async (t) => {
  const originalFetch = globalThis.fetch;
  t.after(() => { globalThis.fetch = originalFetch; });
  const requests = [];
  // 故意让模拟网络忽略取消，以检验响应到达时的取消检查。
  globalThis.fetch = (url, { signal }) => new Promise(resolve => {
    requests.push({ url, signal, resolve });
  });
  let prepared = 0;
  const world = {
    prepare(key) { prepared++; return { key, dispose() {} }; },
    add() {},
  };
  const loader = new SectionLoader(world);
  loader.update(new Vector3(0, 15, 0));
  assert.equal(requests.length, 2);
  assert.equal(requests[0].url, "/api/section?x=0&z=0");
  loader.update(new Vector3(320, 15, 320));
  assert(requests.every(request => request.signal.aborted));
  requests.forEach(request => request.resolve(new Response(fixture)));
  await new Promise(setImmediate);
  assert.equal(prepared, 0);
  loader.update(new Vector3(320, 15, 320));
  assert.equal(requests.length, 4);
  assert.equal(requests[2].url, "/api/section?x=20&z=20");
  loader.dispose();
  requests.slice(2).forEach(request => {
    assert(request.signal.aborted);
    request.resolve(new Response(fixture));
  });
  await new Promise(setImmediate);
  assert.equal(prepared, 0);
  assert.equal(loader.error, "");
});
