import { test } from "node:test";
import assert from "node:assert/strict";
import { PerspectiveCamera } from "three";
import { FlightControls } from "./flight.ts";

test("WASD 忽略俯仰角，升降和偏航方向正确，斜向不会加速", () => {
  class Canvas extends EventTarget {
    captured = false;
    focus() {}
    setPointerCapture() { this.captured = true; }
    hasPointerCapture() { return this.captured; }
  }
  const canvas = new Canvas();
  const camera = new PerspectiveCamera();
  const controls = new FlightControls(camera, canvas);
  canvas.dispatchEvent(Object.assign(new Event("keydown"), { code: "KeyW" }));
  controls.update(1);
  assert.deepEqual(camera.position.toArray(), [0, 0, -16]);
  canvas.dispatchEvent(Object.assign(new Event("keydown"), { code: "Space" }));
  camera.position.set(0, 0, 0);
  controls.update(1);
  assert(Math.abs(camera.position.length() - 16) < 1e-9);
  assert(camera.position.y > 0);
  canvas.dispatchEvent(new Event("blur"));
  canvas.dispatchEvent(Object.assign(new Event("keydown"), { code: "ShiftRight" }));
  camera.position.set(0, 0, 0);
  controls.update(1);
  assert.deepEqual(camera.position.toArray(), [0, -16, 0]);
  canvas.dispatchEvent(new Event("blur"));
  canvas.dispatchEvent(Object.assign(new Event("pointerdown"), { pointerId: 1 }));
  canvas.dispatchEvent(Object.assign(new Event("pointermove"), {
    pointerId: 1,
    movementX: -Math.PI / 2 / 0.003,
    movementY: 0,
  }));
  canvas.dispatchEvent(Object.assign(new Event("keydown"), { code: "KeyW" }));
  camera.position.set(0, 0, 0);
  controls.update(1);
  assert(Math.abs(camera.position.x + 16) < 1e-9);
  assert(Math.abs(camera.position.z) < 1e-9);
  assert.equal(camera.position.y, 0);
});
