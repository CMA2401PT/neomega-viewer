import { test } from "node:test";
import assert from "node:assert/strict";
import * as THREE from "three";
import { FlightControls } from "../src/scene/flight.ts";

test("主相机水平移动、地图输入隔离与监听释放保持独立", () => {
  const previousWindow = globalThis.window;
  globalThis.window = new EventTarget();
  const canvas = new EventTarget();
  canvas.focus = () => {};
  canvas.setPointerCapture = () => {};
  const camera = new THREE.PerspectiveCamera();
  let paused = false, changes = 0;
  const controls = new FlightControls(camera, canvas, {
    paused: () => paused,
    interaction: () => {},
    changed: () => changes++,
  });
  const send = (type, values) => canvas.dispatchEvent(Object.assign(new Event(type, {cancelable:true}), values));
  try {
    controls.reset([0,80,0]);
    controls.rotation.x = 1.4;
    controls.rotation.y = Math.PI / 2;
    send("keydown", {code:"KeyW"});
    controls.update(.25);
    assert.equal(camera.position.y, 80);
    assert.equal(camera.position.x, -6);
    assert.equal(changes, 1);
    paused = true;
    controls.cancel();
    send("keydown", {code:"Space"});
    send("wheel", {deltaY:-1,shiftKey:false});
    controls.update(.25);
    assert.equal(camera.position.y, 80);
    assert.equal(changes, 1);
    paused = false;
    send("keydown", {code:"Space"});
    controls.update(.25);
    assert.equal(camera.position.y, 86);
    controls.dispose();
    send("keydown", {code:"Space"});
    controls.update(.25);
    assert.equal(camera.position.y, 86);
  } finally {
    controls.dispose();
    globalThis.window = previousWindow;
  }
});
