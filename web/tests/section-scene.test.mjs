import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import * as THREE from "three";
import { SectionScene, classifyBox } from "../render/section-scene.ts";
import { decodeFrame, createEffectRegistry } from "../render/index.ts";

const atlas = () => new THREE.Texture({ width: 4096, height: 4096 });
function fixture() {
  const bytes = readFileSync(new URL("./fixtures/block_entities.nvframe", import.meta.url));
  return decodeFrame(bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength));
}
function batches(scene) {
  const result = [];
  scene.traverse((object) => { if (object.isBatchedMesh) result.push(object); });
  return result;
}
function cameraAt(x, y, z, target) {
  const camera = new THREE.PerspectiveCamera(60, 1, 0.1, 100);
  camera.position.set(x, y, z);
  camera.lookAt(...target);
  camera.updateMatrixWorld();
  return camera;
}

test("adjacent sections share GPU batches and distant regions share materials", () => {
  const scene = new THREE.Scene(), texture = atlas();
  const world = new SectionScene(scene, texture, new Map());
  const data = fixture();
  const sections = [{ x: -2, y: 0, z: -1 }, { x: -1, y: 0, z: -1 }, { x: 4, y: 0, z: -1 }]
    .map((key) => world.prepare(key, data));
  const sourcePoint = new THREE.Vector3().fromBufferAttribute(sections[0].opaque[0].geometry.getAttribute("position"), 0);
  world.add(sections[0]);
  const first = batches(scene);
  world.add(sections[1]);
  assert.deepEqual(batches(scene), first);
  assert(first.every((batch) => batch.instanceCount === 2));
  assert(sections[0].opaque.length === 0, "temporary source geometry must be released");
  const matrix = new THREE.Matrix4(), point = new THREE.Vector3();
  first[0].getMatrixAt(0, matrix);
  assert(matrix.equals(new THREE.Matrix4()), "static batching should not need an instance transform");
  const range = first[0].getGeometryRangeAt(0);
  point.fromBufferAttribute(first[0].geometry.getAttribute("position"), range.vertexStart).add(first[0].parent.position);
  assert.deepEqual(point.toArray(), sourcePoint.add(new THREE.Vector3(-32, 0, -16)).toArray());
  world.add(sections[2]);
  const second = batches(scene).filter((batch) => !first.includes(batch));
  assert.equal(second.length, first.length);
  for (const batch of second) assert(first.some((other) => other.material === batch.material));
  const material = first[0].material;
  let disposed = 0;
  material.addEventListener("dispose", () => disposed++);
  sections[0].dispose();
  assert(first.every((batch) => batch.instanceCount === 1));
  assert.equal(disposed, 0);
  world.setWireframe(true);
  assert(batches(scene).every((batch) => batch.material.wireframe));
  world.setWireframe(false);
  assert(batches(scene).every((batch) => !batch.material.wireframe));
  world.dispose();
  assert.equal(scene.children.length, 0);
  assert.equal(disposed, 1);
  texture.dispose();
});

test("hierarchical visibility matches direct section bounds at multiple camera angles", () => {
  const scene = new THREE.Scene(), texture = atlas();
  const world = new SectionScene(scene, texture, new Map());
  const sections = [];
  for (const x of [-8, -7, -1, 0, 1, 6, 7, 8]) {
    const section = world.prepare({ x, y: 0, z: -1 }, fixture());
    world.add(section);
    sections.push(section);
  }
  for (const target of [[0, 2, -16], [-128, 2, -16], [128, 2, -16], [0, 2, 100]]) {
    const camera = cameraAt(0, 12, 40, target);
    const frustum = new THREE.Frustum().setFromProjectionMatrix(
      new THREE.Matrix4().multiplyMatrices(camera.projectionMatrix, camera.matrixWorldInverse),
    );
    world.update(camera, 2);
    const visible = sections.filter((section) => frustum.intersectsBox(section.bounds));
    assert.equal(world.visibility.visibleSections, visible.length);
    for (const section of sections) {
      const expected = visible.includes(section);
      const object = section.frame.object;
      assert.equal(object.visible && object.parent.visible, expected);
    }
    assert(world.visibility.sectionsTested < sections.length);
  }
  // Removing extremal sections must update the region bounds before the next frame.
  sections[0].dispose();
  sections[1].dispose();
  world.update(cameraAt(0, 12, 40, [0, 2, -16]), 3);
  world.dispose();
  assert.equal(scene.children.length, 0);
  texture.dispose();
});

test("box classification handles inside, outside, intersecting and plane-touching bounds", () => {
  const frustum = new THREE.Frustum().setFromProjectionMatrix(new THREE.Matrix4());
  const box = (min, max) => new THREE.Box3(new THREE.Vector3(...min), new THREE.Vector3(...max));
  assert.equal(classifyBox(frustum, box([-.5, -.5, -.5], [.5, .5, .5])), 2);
  assert.equal(classifyBox(frustum, box([1, -.5, -.5], [2, .5, .5])), 1);
  assert.equal(classifyBox(frustum, box([1.01, -.5, -.5], [2, .5, .5])), 0);
  assert.equal(classifyBox(frustum, box([-2, -2, -2], [2, 2, 2])), 1);
});

test("book-only sections animate after attachment, leaving and reentering the view", () => {
  const scene = new THREE.Scene(), texture = atlas(), book = new THREE.Texture();
  scene.matrixWorldAutoUpdate = false;
  const world = new SectionScene(scene, texture, createEffectRegistry(book));
  const empty = { batches: [], quads: 0, bounds: { min: [0, 0, 0], max: [0, 0, 0] } };
  const section = world.prepare({ x: -1, y: 2, z: 0 }, {
    mesh: empty, actors: [], effects: [{ pos: [3, 1, 3], effect: "enchanting_table_book" }],
  });
  world.setWireframe(true);
  world.add(section);
  section.frame.object.traverse((object) => {
    if (object.material) assert.equal(object.material.wireframe, true);
  });
  world.setWireframe(false);
  assert.equal(batches(scene).length, 0);
  const camera = cameraAt(-12, 40, 20, [-12, 34, 3]);
  world.update(camera, 1);
  const effect = section.frame.object.children.at(-1);
  const before = effect.matrixWorld.clone();
  world.update(camera, 3);
  assert(!effect.matrixWorld.equals(before));
  assert(section.bounds.containsBox(new THREE.Box3().setFromObject(effect)));
  camera.lookAt(-12, 40, 80);
  world.update(camera, 4);
  camera.lookAt(-12, 34, 3);
  world.update(camera, 10);
  assert.equal(effect.rotation.y, 3);
  world.dispose();
  texture.dispose();
  book.dispose();
});

test("regional buffers shrink after unloading and reuse slots without disturbing remaining sections", () => {
  const scene = new THREE.Scene(), texture = atlas();
  const world = new SectionScene(scene, texture, new Map());
  const data = fixture(), sections = [];
  for (let x = 0; x < 4; x++)
    for (let y = 0; y < 4; y++)
      for (let z = 0; z < 4; z++) {
        const section = world.prepare({ x, y, z }, data);
        world.add(section);
        sections.push(section);
      }
  const batch = batches(scene)[0];
  assert.equal(batch.instanceCount, 64);
  const previousCapacity = batch.geometry.getAttribute("position").count;
  const previousRange = batch.getGeometryRangeAt(63);
  const previousPoint = new THREE.Vector3().fromBufferAttribute(batch.geometry.getAttribute("position"), previousRange.vertexStart);
  sections.slice(0, -1).forEach((section) => section.dispose());
  world.update(cameraAt(55, 60, 80, [55, 50, 55]), 1);
  assert.equal(batch.instanceCount, 1);
  assert(batch.geometry.getAttribute("position").count < previousCapacity);
  const compacted = batch.getGeometryRangeAt(63);
  assert.deepEqual(new THREE.Vector3().fromBufferAttribute(batch.geometry.getAttribute("position"), compacted.vertexStart).toArray(), previousPoint.toArray());
  const replacement = world.prepare({ x: 0, y: 0, z: 0 }, data);
  world.add(replacement);
  assert.equal(batch.instanceCount, 2);
  assert(batch.getVisibleAt(0));
  world.dispose();
  assert.equal(scene.children.length, 0);
  texture.dispose();
});
