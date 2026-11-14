import { test } from "node:test";
import assert from "node:assert/strict";
import { mapPoint, mapScreen, horizontalBounds } from "../src/minimap/map-camera.ts";
import { RegionDownloads, windowOrigin } from "../src/terrain/map-regions.ts";
import { MapTiles } from "../src/minimap/map-tiles.ts";

const tick = () => new Promise(resolve => setImmediate(resolve));
const tile = x => ({ x, z: 0, kind: "overview", key: `overview/${x}/0` });

test("旋转后拖动和缩放仍以鼠标所在水平点为锚，倾角计入窗口覆盖范围", () => {
  const camera = { target: [-200, 80, 50], yaw: 35, pitch: 65, span: 200 };
  const point = mapPoint(camera, 400, 200, 37, 62);
  const screen = mapScreen(camera, 400, 200, point);
  assert.ok(Math.abs(screen[0] - 37) < 1e-9 && Math.abs(screen[1] - 62) < 1e-9);
  const zoomed = { ...camera, target: [...camera.target], span: 100 };
  const shifted = mapPoint(zoomed, 400, 200, 37, 62);
  zoomed.target = zoomed.target.map((n, i) => n + point[i] - shifted[i]);
  const anchored = mapPoint(zoomed, 400, 200, 37, 62);
  assert.ok(anchored.every((n, i) => Math.abs(n - point[i]) < 1e-9));
  const high = horizontalBounds({...camera,yaw:0,pitch:85},400,200,-64,448);
  const low = horizontalBounds({...camera,yaw:0,pitch:40},400,200,-64,448);
  assert.ok(low[3]-low[1] > high[3]-high[1]+400);
  assert.deepEqual(windowOrigin(-65,63),{x:-7,z:-6});
  assert.deepEqual(windowOrigin(-63,65),{x:-6,z:-5});
});

test("窗口移动保留重叠区域的迟到响应，离开的实例无法重新发布", async () => {
  const pending=[];
  const queue=new RegionDownloads(assert.fail,(revision,position,signal)=>new Promise(resolve=>pending.push({revision,position,signal,resolve})));
  queue.reset(7);
  queue.update([{x:0,z:0},{x:1,z:0}]);
  const kept=queue.entries.get("1/0");
  queue.update([{x:1,z:0},{x:2,z:0}]);
  assert.equal(pending[0].signal.aborted,true);
  assert.equal(pending.length,2);
  pending[0].resolve({old:true});
  pending[1].resolve({kept:true});
  await tick();
  queue.update([{x:1,z:0},{x:2,z:0}]);
  assert.equal(pending.length,3);
  assert.equal(queue.entries.get("1/0"),kept);
  assert.deepEqual(kept.volume,{kept:true});
  assert.equal(queue.entries.has("0/0"),false);
});

test("切换存档后同坐标使用新实例，旧请求退出前仍占下载名额", async () => {
  const pending=[];
  const queue=new RegionDownloads(assert.fail,(revision,position,signal)=>new Promise(resolve=>pending.push({revision,signal,resolve})));
  queue.reset(1);
  queue.update([{x:0,z:0},{x:1,z:0}]);
  queue.reset(2);
  queue.update([{x:0,z:0}]);
  assert.equal(pending.length,2);
  pending[0].resolve({old:true});
  pending[1].resolve({old:true});
  await tick();
  queue.update([{x:0,z:0}]);
  assert.equal(pending[2].revision,2);
  assert.equal(queue.entries.get("0/0").volume,undefined);
  pending[2].resolve({new:true});
  await tick();
  assert.deepEqual(queue.entries.get("0/0").volume,{new:true});
});

test("占用图切换代次释放旧图片，同名旧请求完成后才能补入新请求", async () => {
  const pending=[];
  let released=0;
  const cache=new MapTiles((entry,signal)=>new Promise(resolve=>pending.push({signal,resolve})),assert.fail);
  cache.update([tile(0)]);
  cache.reset();
  cache.update([tile(0)]);
  assert.equal(pending[0].signal.aborted,true);
  pending[0].resolve({close(){released++;}});
  await tick();
  assert.equal(cache.images.size,0);
  pending[1].resolve({close(){released++;}});
  await tick();
  assert.equal(cache.images.size,1);
  cache.reset();
  assert.equal(released,2);
});
