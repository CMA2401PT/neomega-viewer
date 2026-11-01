import { test } from "node:test";
import assert from "node:assert/strict";
import * as THREE from "three";
import { OcclusionQueries, fullyInside } from "../render/occlusion.ts";

function setup() {
  const queries = [], deleted = [];
  const gl = {
    QUERY_RESULT_AVAILABLE: 1, QUERY_RESULT: 2,
    isContextLost: () => false,
    createQuery: () => { const q = { ready: false, visible: false }; queries.push(q); return q; },
    deleteQuery: (q) => deleted.push(q),
    getQueryParameter: (q, param) => param === 1 ? q.ready : q.visible,
    beginQuery() {}, endQuery() {},
  };
  const renderer = { getContext: () => gl, getDrawingBufferSize: (out) => out.set(800, 600), render() {} };
  const culler = new OcclusionQueries(renderer);
  const camera = new THREE.PerspectiveCamera(60, 1, 0.1, 100);
  camera.updateMatrixWorld();
  const frustum = new THREE.Frustum().setFromProjectionMatrix(camera.projectionMatrix);
  const target = () => ({
    bounds: new THREE.Box3(new THREE.Vector3(-1, -1, -20), new THREE.Vector3(1, 1, -18)),
  });
  culler.begin(camera);
  culler.begin(camera);
  return { culler, camera, frustum, target, queries, deleted };
}

test("occlusion is asynchronous, bounded, and cached across rotation but not translation", () => {
  const { culler, camera, frustum, target, queries, deleted } = setup();
  const targets = Array.from({ length: 20 }, target);
  assert(culler.prepare(targets, frustum));
  assert.equal(queries.length, 8);
  culler.begin(camera);
  assert.equal(culler.stats.pending, 8);
  assert(!culler.hidden(targets[0]), "unavailable results must stay visible");
  queries.forEach((q) => q.ready = true);
  queries[1].visible = true;
  camera.rotation.y = 0.2;
  camera.updateMatrixWorld();
  culler.begin(camera);
  assert(culler.hidden(targets[0]));
  assert(!culler.hidden(targets[1]));
  assert.equal(deleted.length, 8);
  assert(!culler.prepare(targets.slice(0, 8), frustum), "stable regions need no recurring queries");
  culler.begin(camera);
  culler.begin(camera);
  assert(!culler.hidden(targets[0]), "recheck subpixel gaps after rotation settles");
  culler.prepare([targets[0]], frustum);
  queries.at(-1).ready = true;
  culler.begin(camera);
  assert(culler.hidden(targets[0]));
  camera.position.x = 0.01;
  camera.updateMatrixWorld();
  culler.begin(camera);
  assert(!culler.hidden(targets[0]));
  assert(!culler.prepare(targets, frustum), "skip queries while translating");
  culler.dispose();
});

test("screen-edge and near-plane boxes stay visible; geometry changes discard pending results", () => {
  const { culler, camera, frustum, target, queries, deleted } = setup();
  const clipped = target();
  clipped.bounds.max.x = 50;
  const near = target();
  near.bounds.max.z = 1;
  assert(!fullyInside(frustum, clipped.bounds));
  assert(!fullyInside(frustum, near.bounds));
  assert(!culler.prepare([clipped, near], frustum));
  const visible = target();
  assert(culler.prepare([visible], frustum));
  culler.invalidate();
  queries[0].ready = true;
  culler.begin(camera);
  assert(!culler.hidden(visible));
  assert.deepEqual(deleted, queries);
  assert(culler.prepare([visible], frustum));
  culler.dispose();
  assert.equal(deleted.length, 2);
});

test("projection changes invalidate cached occlusion", () => {
  const { culler, camera, frustum, target, queries } = setup();
  const object = target();
  culler.prepare([object], frustum);
  queries[0].ready = true;
  culler.begin(camera);
  assert(culler.hidden(object));
  camera.aspect = 2;
  camera.updateProjectionMatrix();
  culler.begin(camera);
  assert(!culler.hidden(object));
  culler.dispose();
});
