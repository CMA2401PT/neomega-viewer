import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { decodeMesh } from "../src/render/binary.ts";
import { createMeshObject } from "../src/render/mesh.ts";
import { Texture } from "three";

function fixture() {
  const metadata = new TextEncoder().encode(
    JSON.stringify([
      {
        texture: "minecraft:block/stone",
        region: [0, 0, 1, 1],
        mode: "opaque",
        doubleSided: false,
        unlit: false,
        opacity: 1,
      },
    ]),
  );
  const start = Math.ceil((44 + metadata.length) / 4) * 4;
  const result = new ArrayBuffer(start + 8 + (4 * 14 + 6) * 4);
  const view = new DataView(result);
  new Uint8Array(result).set(new TextEncoder().encode("NVMS"));
  view.setUint32(4, 1, true);
  view.setUint32(8, metadata.length, true);
  view.setUint32(12, 1, true);
  view.setUint32(16, 1, true);
  view.setFloat32(32, 1, true);
  view.setFloat32(36, 0.5, true);
  new Uint8Array(result, 44, metadata.length).set(metadata);
  view.setUint32(start, 4, true);
  view.setUint32(start + 4, 6, true);
  let offset = start + 8;
  const arrays = [
    [0, 0, 0, 0, 0.5, 0, 1, 0.5, 0, 1, 0, 0],
    [0, 0, -1, 0, 0, -1, 0, 0, -1, 0, 0, -1],
    [0, 0, -1, 0, 0, -1, 0, 0, -1, 0, 0, -1],
    [0, 0, 0, 0.5, 2, 0.5, 2, 0],
    [1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1],
  ];
  for (const values of arrays)
    for (const v of values) {
      view.setFloat32(offset, v, true);
      offset += 4;
    }
  for (const v of [0, 1, 2, 0, 2, 3]) {
    view.setUint32(offset, v, true);
    offset += 4;
  }
  return result;
}

test("fractional UVs and vertices share the received buffer", () => {
  const input = fixture();
  const mesh = decodeMesh(input);
  assert.equal(mesh.quads, 1);
  assert.deepEqual(mesh.bounds, { min: [0, 0, 0], max: [1, 0.5, 0] });
  assert(mesh.batches[0].positions instanceof Float32Array);
  assert(mesh.batches[0].indices instanceof Uint32Array);
  assert.equal(mesh.batches[0].positions.buffer, input);
  assert.deepEqual(
    Array.from(mesh.batches[0].uvs),
    [0, 0, 0, 0.5, 2, 0.5, 2, 0],
  );
});

test("reject truncated payload, unknown layout and invalid indices", () => {
  const input = fixture();
  assert.throws(() => decodeMesh(input.slice(0, input.byteLength - 1)));
  new DataView(input).setUint32(4, 2, true);
  assert.throws(() => decodeMesh(input));
  const badIndex = fixture();
  new DataView(badIndex).setUint32(badIndex.byteLength - 4, 4, true);
  assert.throws(() => decodeMesh(badIndex));
});

test(
  "decode the Go-generated solid subchunk fixture",
  { skip: !process.env.NEOMEGA_MESH_FIXTURE },
  () => {
    const bytes = readFileSync(process.env.NEOMEGA_MESH_FIXTURE);
    const buffer = bytes.buffer.slice(
      bytes.byteOffset,
      bytes.byteOffset + bytes.byteLength,
    );
    const mesh = decodeMesh(buffer);
    assert.equal(mesh.quads, 6);
    assert(mesh.bounds.min.every((v) => Math.abs(v) < 1e-6));
    assert(mesh.bounds.max.every((v) => Math.abs(v - 16) < 1e-6));
    assert.equal(
      mesh.batches.reduce((sum, b) => sum + b.positions.length / 3, 0),
      24,
    );
    assert(
      mesh.batches.some((batch) => batch.uvs.some((uv) => Math.abs(uv) > 1)),
    );
  },
);

test("Three geometry retains the typed views and leaves the shared atlas owned by the caller", () => {
  const input = fixture();
  const data = decodeMesh(input);
  const atlas = new Texture({ width: 16, height: 16 });
  let disposed = 0;
  atlas.addEventListener("dispose", () => disposed++);
  const mesh = createMeshObject(data, atlas);
  const draw = mesh.object.children[0];
  assert.equal(draw.geometry.getAttribute("position").array.buffer, input);
  assert.equal(draw.geometry.getAttribute("uv").array.buffer, input);
  assert.equal(draw.geometry.getIndex().array, data.batches[0].indices);
  mesh.dispose();
  assert.equal(disposed, 0);
  atlas.dispose();
  assert.equal(disposed, 1);
});
