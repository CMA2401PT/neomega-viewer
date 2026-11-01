import { test } from "node:test";
import assert from "node:assert/strict";
import * as THREE from "three";
import { FrameRate, geometryMemoryBytes } from "../src/metrics.ts";

test("memory includes reserved storage without double-counting shared CPU and GPU buffers", () => {
  const storage = new Float32Array(48);
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute("position", new THREE.BufferAttribute(storage.subarray(0, 24), 3));
  geometry.setAttribute("color", new THREE.BufferAttribute(storage.subarray(24), 3));
  geometry.setIndex(new THREE.BufferAttribute(new Uint16Array([0, 1, 2]), 1));
  geometry.setDrawRange(0, 3);
  const scene = new THREE.Scene(), material = new THREE.MeshBasicMaterial();
  const first = new THREE.Mesh(geometry, material), second = new THREE.Mesh(geometry, material);
  scene.add(first, second);
  assert.equal(geometryMemoryBytes(scene), 2 * (48 * 4 + 3 * 2));
  scene.remove(first);
  assert.equal(geometryMemoryBytes(scene), 2 * (48 * 4 + 3 * 2));
  scene.remove(second);
  assert.equal(geometryMemoryBytes(scene), 0);
  geometry.dispose();
  material.dispose();
});

test("interleaved attributes share one GPU buffer", () => {
  const data = new THREE.InterleavedBuffer(new Float32Array(24), 6);
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute("position", new THREE.InterleavedBufferAttribute(data, 3, 0));
  geometry.setAttribute("color", new THREE.InterleavedBufferAttribute(data, 3, 3));
  const material = new THREE.MeshBasicMaterial();
  assert.equal(geometryMemoryBytes(new THREE.Mesh(geometry, material)), 2 * 24 * 4);
  geometry.dispose();
  material.dispose();
});

test("FPS uses elapsed wall time and excludes time spent in a hidden page after reset", () => {
  const rate = new FrameRate();
  for (const time of [0, 250, 500, 750]) assert.equal(rate.sample(time), undefined);
  assert.equal(rate.sample(1000), 4);
  assert.equal(rate.sample(1250), undefined);
  assert.equal(rate.sample(2250), 1.6);
  rate.reset();
  assert.equal(rate.sample(10000), undefined);
  assert.equal(rate.sample(10500), undefined);
  assert.equal(rate.sample(11000), 2);
});
