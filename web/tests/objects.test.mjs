import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import * as THREE from "three";
import {
  createBatchObject,
  createEffectsObject,
  createEffectRegistry,
  decodeFrame,
  SurfaceMaterials,
} from "@neomega/viewer/render";

test("filtered batches derive their own bounds and release buffers without shared materials", () => {
  const bytes = readFileSync(new URL("./fixtures/block_entities.nvframe", import.meta.url));
  const buffer = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
  const data = decodeFrame(buffer);
  const batches = data.mesh.batches.filter((batch) => batch.surface.mode === "blend");
  const atlas = new THREE.Texture({ width: 4096, height: 4096 });
  const materials = new SurfaceMaterials(atlas);
  const draw = createBatchObject(
    batches,
    atlas,
    {
      materials,
      copyBuffers: true,
      sortTransparentQuads: false,
    },
  );
  const expected = new THREE.Box3();
  for (const batch of batches) {
    for (let offset = 0; offset < batch.positions.length; offset += 3)
      expected.expandByPoint(new THREE.Vector3().fromArray(batch.positions, offset));
  }
  assert(draw.bounds.equals(expected));
  assert.equal(draw.object.children.length, batches.length);
  assert.equal(draw.animated, batches.some((batch) => batch.surface.frames > 1));
  draw.updateTime(4.25);
  assert.equal(materials.time.value, 4.25);
  let buffersDisposed = 0, materialsDisposed = 0, texturesDisposed = 0;
  atlas.addEventListener("dispose", () => texturesDisposed++);
  for (const mesh of draw.object.children) {
    mesh.geometry.addEventListener("dispose", () => buffersDisposed++);
    mesh.material.addEventListener("dispose", () => materialsDisposed++);
    for (const attribute of Object.values(mesh.geometry.attributes))
      assert.notEqual(attribute.array.buffer, buffer);
  }
  draw.dispose();
  assert.equal(buffersDisposed, batches.length);
  assert.equal(materialsDisposed, 0);
  assert.equal(texturesDisposed, 0);
  materials.dispose();
  assert.equal(materialsDisposed, batches.length);
  atlas.dispose();
});

test("effects animate independently with motion bounds and caller-owned textures", () => {
  const texture = new THREE.Texture();
  const registry = createEffectRegistry(texture);
  const draw = createEffectsObject(
    [
      { pos: [3, 1, 3], effect: "enchanting_table_book" },
      { pos: [30, 1, 3], effect: "unknown" },
    ],
    registry,
  );
  assert(draw.object instanceof THREE.Group);
  assert.equal(draw.object.children.length, 1);
  assert.equal(draw.animated, true);
  const book = draw.object.children[0];
  draw.updateTime(10);
  assert.equal(book.rotation.y, 3);
  assert(draw.bounds.containsBox(new THREE.Box3().setFromObject(book)));
  const geometries = new Set(), materials = new Set();
  draw.object.traverse((object) => {
    if (!object.isMesh) return;
    geometries.add(object.geometry);
    materials.add(object.material);
  });
  let geometryDisposals = 0, materialDisposals = 0, textureDisposals = 0;
  geometries.forEach((geometry) => geometry.addEventListener("dispose", () => geometryDisposals++));
  materials.forEach((material) => material.addEventListener("dispose", () => materialDisposals++));
  texture.addEventListener("dispose", () => textureDisposals++);
  draw.dispose();
  assert.equal(geometryDisposals, geometries.size);
  assert.equal(materialDisposals, materials.size);
  assert.equal(textureDisposals, 0);
  texture.dispose();
});

test("empty primitives keep empty bounds when composed with other render objects", () => {
  const atlas = new THREE.Texture();
  const batches = createBatchObject([], atlas);
  const effects = createEffectsObject([], new Map());
  for (const draw of [batches, effects]) {
    assert(draw.bounds.isEmpty());
    assert.equal(draw.object.children.length, 0);
    assert.equal(draw.animated, false);
    draw.dispose();
  }
  atlas.dispose();
});
