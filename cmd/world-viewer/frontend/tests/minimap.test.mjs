import { test } from "node:test";
import assert from "node:assert/strict";
import { mapPoint, visibleTiles, MapTiles } from "../src/minimap.ts";

const tick = () => new Promise((resolve) => setImmediate(resolve));
const tile = (x, kind = "detail") => ({ x, z: 0, kind, key: `${kind}/${x}/0` });

test("地图采用北上坐标，负坐标图块按向下取整定位，近中心优先", () => {
  assert.deepEqual(mapPoint({ x: -128, z: 0 }, 2, 200, 100, 0, 0), { x: -328, z: -100 });
  const index = { detail: [{ x: -2, z: 0 }, { x: -1, z: 0 }, { x: 0, z: 0 }], overview: [{ x: -1, z: 0 }] };
  assert.deepEqual(visibleTiles({ x: -1, z: 1 }, 1, 100, 100, index).map((entry) => entry.x), [-1, 0]);
  assert.equal(visibleTiles({ x: -1, z: 1 }, 1, 100, 100).length, 4);
  assert.deepEqual(visibleTiles({ x: 0, z: 0 }, 8, 100, 100), []);
  assert.equal(visibleTiles({ x: 0, z: 0 }, 8, 100, 100, index)[0].kind, "overview");
  assert.equal(visibleTiles({ x: 0, z: 0 }, 4, 1200, 800, index)[0].kind, "overview");
});

test("平移取消离开视口的下载，收起与展开复用已加载图片", async () => {
  const pending = [];
  const cache = new MapTiles((entry, signal) => new Promise((resolve) => pending.push({ entry, signal, resolve })), () => {}, assert.fail);
  cache.update([tile(0), tile(1), tile(2)]);
  assert.equal(pending.length, 2);
  cache.update([tile(1), tile(2)]);
  assert.equal(pending[0].signal.aborted, true);
  let released = 0;
  pending[0].resolve({ close() { released++; } });
  pending[1].resolve({ close() { released++; } });
  await tick();
  assert.equal(released, 1);
  assert.equal(pending.length, 3);
  cache.update([tile(1)]);
  assert.equal(pending[2].signal.aborted, true);
  pending[2].resolve({ close() { released++; } });
  await tick();
  cache.update([tile(1)]);
  assert.equal(pending.length, 3);
  assert.equal(cache.images.size, 1);
  cache.reset();
  assert.equal(released, 3);
});

test("切换存档释放缓存并丢弃迟到图片，同名图块的新请求不受旧 finally 影响", async () => {
  const pending = [];
  let released = 0;
  const cache = new MapTiles((entry, signal) => new Promise((resolve) => pending.push({ signal, resolve })), () => {}, assert.fail);
  cache.update([tile(0)]);
  cache.reset();
  cache.update([tile(0)]);
  assert.equal(pending[0].signal.aborted, true);
  pending[0].resolve({ close() { released++; } });
  await tick();
  assert.equal(cache.images.size, 0);
  pending[1].resolve({ close() { released++; } });
  await tick();
  assert.equal(cache.images.size, 1);
  assert.equal(released, 1);
  cache.reset();
  assert.equal(released, 2);
});
