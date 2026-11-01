import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { Box3, Texture } from "three";
import { decodeFrame, createFrameObject, createEffectRegistry } from "@neomega/viewer/render";

function fixture() {
  const bytes = readFileSync(new URL("./fixtures/block_entities.nvframe", import.meta.url));
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
}
const emptyMesh = { batches: [], quads: 0, bounds: { min: [0, 0, 0], max: [0, 0, 0] } };

test("Go NVFR fixture contains static geometry, one book and six positioned actors", () => {
  const buffer = fixture();
  const frame = decodeFrame(buffer);
  assert.equal(frame.mesh.quads, 19);
  assert.deepEqual(frame.mesh.bounds.min, [1, 0, 1]);
  assert.deepEqual(frame.effects, [{ pos: [3, 1, 3], effect: "enchanting_table_book" }]);
  assert.deepEqual(frame.actors.map((actor) => actor.pos), [
    [3, 1, 7], [5, 1, 11], [6, 1, 7], [9, 1, 7], [10, 1, 11], [12, 1, 7],
  ]);
  const textures = frame.mesh.batches.map((batch) => batch.surface.texture);
  assert(textures.includes("minecraft:block/enchanting_table_top"));
  assert(textures.includes("minecraft:block/water_still"));
  assert(!textures.some((name) => name.includes("decorated_pot")));
  for (const actor of frame.actors) {
    assert.equal(actor.mesh.quads, 18);
    assert.equal(actor.mesh.bounds.min[0], actor.pos[0] + 1 / 16);
    assert.equal(actor.mesh.bounds.max[1], actor.pos[1] + 1.25);
    const patterns = actor.mesh.batches.filter((batch) => batch.surface.texture.includes("pottery_pattern"));
    assert.equal(patterns.length, actor.pos[0] === 5 ? 0 : 4);
  }
  for (const mesh of [frame.mesh, ...frame.actors.map((actor) => actor.mesh)]) {
    for (const batch of mesh.batches) {
      assert.equal(batch.positions.buffer, buffer);
      assert.equal(batch.indices.buffer, buffer);
    }
  }
});

test("NVFR uses directory types and bounded NVMS payloads", () => {
  const buffer = fixture();
  const directory = new Uint8Array(buffer, 12, 36);
  const first = directory.slice(0, 12);
  directory.copyWithin(0, 24, 36);
  directory.set(first, 24);
  assert.equal(decodeFrame(buffer).actors.length, 6);
  assert.throws(() => decodeFrame(buffer.slice(0, buffer.byteLength - 4)));
  const wrongVersion = fixture();
  new DataView(wrongVersion).setUint32(4, 2, true);
  assert.throws(() => decodeFrame(wrongVersion), /NVFR 版本/);
  const extraMeshBytes = fixture();
  const view = new DataView(extraMeshBytes);
  view.setUint32(20, view.getUint32(20, true) + 4, true);
  assert.throws(() => decodeFrame(extraMeshBytes), /网格长度/);
});

test("frame objects combine bounds and retain baked actor coordinates", () => {
  const data = decodeFrame(fixture());
  const atlas = new Texture({ width: 4096, height: 4096 });
  const book = new Texture();
  const frame = createFrameObject(data, atlas, createEffectRegistry(book));
  assert.equal(frame.object.children.length, 8);
  assert.deepEqual(frame.object.children[1].position.toArray(), [0, 0, 0]);
  assert.equal(frame.bounds.max.y, 2.65);
  assert.deepEqual(frame.bounds.min.toArray(), [1, 0, 1]);
  const geometries = new Set();
  const materials = new Set();
  frame.object.traverse((draw) => {
    if (draw.isMesh) {
      geometries.add(draw.geometry);
      materials.add(draw.material);
    }
  });
  let geometryDisposals = 0;
  let materialDisposals = 0;
  let textureDisposals = 0;
  geometries.forEach((geometry) => geometry.addEventListener("dispose", () => geometryDisposals++));
  materials.forEach((material) => material.addEventListener("dispose", () => materialDisposals++));
  atlas.addEventListener("dispose", () => textureDisposals++);
  book.addEventListener("dispose", () => textureDisposals++);
  frame.updateTime(2.5);
  frame.object.traverse((draw) => {
    if (draw.isMesh && draw.material.isShaderMaterial)
      assert.equal(draw.material.uniforms.time.value, 2.5);
  });
  frame.dispose();
  assert.equal(geometryDisposals, geometries.size);
  assert.equal(materialDisposals, materials.size);
  assert.equal(textureDisposals, 0);
  atlas.dispose();
  book.dispose();
});

test("book-only frames animate without atlas animation and include their full motion bounds", () => {
  const atlas = new Texture({ width: 16, height: 16 });
  const texture = new Texture();
  const registry = createEffectRegistry(texture);
  const message = { pos: [3, 1, 3], effect: "enchanting_table_book" };
  const frame = createFrameObject({ mesh: emptyMesh, actors: [], effects: [message] }, atlas, registry);
  assert.equal(frame.animated, true);
  const book = frame.object.children[1];
  const start = book.rotation.y;
  for (let time = 0; time < 24; time += 0.25) {
    frame.updateTime(time);
    assert(frame.bounds.containsBox(new Box3().setFromObject(book)));
    assert(book.position.y > 1.75 + 0.25);
  }
  assert.notEqual(book.rotation.y, start);
  frame.dispose();
  const unknown = createFrameObject({ mesh: emptyMesh, actors: [], effects: [{ ...message, effect: "unknown" }] }, atlas, registry);
  assert.equal(unknown.animated, false);
  assert.deepEqual(unknown.bounds.min.toArray(), [0, 0, 0]);
  assert.deepEqual(unknown.bounds.max.toArray(), [1, 1, 1]);
  unknown.dispose();
  atlas.dispose();
  texture.dispose();
});
