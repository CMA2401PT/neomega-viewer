import { test } from "node:test";
import assert from "node:assert/strict";
import { BrickmapCoverage } from "../../../../../../neomega-viewer-core/viewer/brickmap/web/coverage.ts";
import { BrickmapPool, BrickmapWindow, directoryLevels, mergeParents, UNKNOWN, MIXED } from "../../../../../../neomega-viewer-core/viewer/brickmap/web/gpu.ts";

function fakeGL() {
  let id=0;
  const writes=[];
  const gl=new Proxy({writes, getParameter:()=>256, createTexture:()=>({id:++id}), texSubImage3D:(...args)=>writes.push(args)}, {
    get(target,key) { return key in target ? target[key] : typeof key === "string" && key.toUpperCase() === key ? key : ()=>{}; }
  });
  return gl;
}
function volume(origin, reverse=false) {
  const levels=directoryLevels([128,128,128]);
  levels[0].cells.fill(0);
  levels[0].cells[0]=65536;
  levels[0].cells[1]=reverse?1:2;
  mergeParents(levels,1);
  const palette=new Uint8Array(72);
  for(let side=0;side<6;side++) {
    palette.set([200,10,20,255],(reverse?2:1)*24+side*4);
    palette.set([20,30,200,100],(reverse?1:2)*24+side*4);
  }
  const groups=reverse?new Uint32Array([0,8,0]):new Uint32Array([0,0,8]);
  const bricks=new Uint16Array(512).fill(reverse?2:1);
  bricks[0]=reverse?1:2;
  return {origin,size:[128,128,128],levels,palette,groups,bricks};
}

test("不同区域局部编号按六面材质和 Group 合并，窗口跳动保留槽位与原始目录",()=>{
  const gl=fakeGL();
  const pool=new BrickmapPool(gl,8*1024**2);
  const left=volume([0,0,0]),right=volume([128,0,0],true);
  const a=pool.upload(left),b=pool.upload(right);
  assert.equal(a.materials[1],b.materials[2]);
  assert.equal(a.materials[2],b.materials[1]);
  const win=new BrickmapWindow(gl,[0,0,0],[1024,128,1024]);
  win.install(a);win.install(b);
  assert.equal(win.levels[0].cells[0],65536+a.slots[0]);
  assert.equal(win.levels[0].cells[16],65536+b.slots[0]);
  assert.equal(win.levels[0].cells[17],b.materials[1]);
  assert.equal(win.levels[0].cells[32],UNKNOWN);
  assert.equal(win.levels.at(-1).cells[0],MIXED);
  right.levels[0].cells.fill(99);
  win.rebuild([128,0,0],[b]);
  assert.equal(win.levels[0].cells[0],65536+b.slots[0]);
  assert.equal(b.levels[0].cells[0],65536);
  assert.equal(win.levels[0].cells[16],UNKNOWN);
  pool.release(a);
  const replacement=pool.upload(volume([256,0,0]));
  assert.equal(replacement.slots[0],a.slots[0]);
  win.remove(b);
  assert.equal(win.levels.at(-1).cells[0],UNKNOWN);
});

test("砖池不足不发布区域也不消耗剩余槽位，归还后可重新安装",()=>{
  const gl=fakeGL();
  const pool=new BrickmapPool(gl,8*1024**2);
  const source=volume([0,0,0]);
  source.size=[128,256,128];
  source.levels=directoryLevels(source.size);
  source.levels[0].cells.set(Uint32Array.from({length:8192},(_,i)=>65536+i));
  mergeParents(source.levels,1);
  source.bricks=new Uint16Array(8192*512).fill(1);
  for(let i=0;i<source.bricks.length;i+=512)source.bricks[i]=2;
  const full=pool.upload(source);
  assert.ok(full);
  const before=gl.writes.length;
  assert.equal(pool.upload(volume([128,0,0])),null);
  assert.equal(gl.writes.length,before);
  pool.release(full);
  assert.ok(pool.upload(volume([128,0,0])));
});

test("1536 格窗口保留最后一个区域，补齐目录不扩大实际范围", () => {
  const gl = fakeGL();
  const pool = new BrickmapPool(gl, 8 * 1024 ** 2);
  const region = pool.upload(volume([1408, 0, 1408]));
  const win = new BrickmapWindow(gl, [0, 0, 0], [1536, 128, 1536]);
  win.install(region);
  assert.deepEqual(win.size, [1536, 128, 1536]);
  assert.deepEqual(win.levels[0].size, [256, 16, 256]);
  assert.deepEqual(win.levels.at(-1).size, [1, 1, 1]);
  const leaf = win.levels[0].cells;
  assert.equal(leaf[176 + 256 * 16 * 176], 65536 + region.slots[0]);
  assert.equal(leaf[192 + 256 * 16 * 176], UNKNOWN);
  assert.equal(win.levels[4].cells[11 + 16 * 11], MIXED);
  assert.equal(win.levels[7].cells[1 + 2], MIXED);
  assert.equal(win.levels.at(-1).cells[0], MIXED);
  win.rebuild([128, 0, 128], [region]);
  assert.equal(win.levels[0].cells[160 + 256 * 16 * 160], 65536 + region.slots[0]);
  win.remove(region);
  assert.equal(win.levels.at(-1).cells[0], UNKNOWN);
});

test("精细覆盖按世界 section 定位，卸载后取消远景避让", () => {
  const gl = fakeGL();
  const coverage = new BrickmapCoverage(gl, [1536, 512, 1536]);
  coverage.update([-768, -64, -768], [{x:-48,y:-4,z:-48}, {x:47,y:27,z:47}, {x:48,y:0,z:0}]);
  const cells = gl.writes.at(-1).at(-1);
  assert.equal(cells[0], 1);
  assert.equal(cells.at(-1), 1);
  assert.equal(cells.reduce((a,b) => a+b, 0), 2);
  assert.deepEqual(coverage.min, [0,0,0]);
  assert.deepEqual(coverage.max, [1536,512,1536]);
  coverage.update([-768,-64,-768], []);
  assert.ok(gl.writes.at(-1).at(-1).every(n => n === 0));
  assert.deepEqual(coverage.min, [0,0,0]);
  assert.deepEqual(coverage.max, [0,0,0]);
});
