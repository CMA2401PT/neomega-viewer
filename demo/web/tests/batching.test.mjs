import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { Texture } from "three";
import { decodeFrame } from "../../../web/render/binary.ts";
import { createMeshObject } from "../../../web/render/mesh.ts";
import { opaqueBatchGroups } from "../../../web/render/batching.ts";

// Compare the actual indexed vertex stream, including per-face texture selection.
function vertices(object) {
  const result = [];
  object.traverse((draw) => {
    if (!draw.isMesh) return;
    const { geometry, material } = draw;
    for (const index of geometry.index.array) {
      const vertex = {};
      for (const name of ["position", "shadeNormal", "color", "uv"]) {
        const attribute = geometry.getAttribute(name);
        vertex[name] = Array.from(attribute.array.slice(index * attribute.itemSize, (index + 1) * attribute.itemSize));
      }
      const region = geometry.getAttribute("atlasRegion");
      vertex.region = region
        ? Array.from(region.array.slice(index * 4, index * 4 + 4))
        : material.uniforms.region.value.toArray().map(Math.fround);
      vertex.state = [material.transparent, material.side, ...["frames", "frameTime", "opacity", "cutoff", "unlit"].map((name) => material.uniforms[name].value)];
      result.push(JSON.stringify(vertex));
    }
  });
  return result.sort();
}

test("batching preserves real frame vertices, UVs, colors, textures and material state", () => {
  const bytes = readFileSync(new URL("./fixtures/block_entities.nvframe", import.meta.url));
  const source = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
  const frame = decodeFrame(source);
  const atlas = new Texture({ width: 4096, height: 4096 });
  let before = 0, after = 0;
  for (const data of [frame.mesh, ...frame.actors.map((actor) => actor.mesh)]) {
    const original = createMeshObject(data, atlas, { sortTransparentQuads: false });
    const batched = createMeshObject(data, atlas, { sortTransparentQuads: false, batchOpaqueSurfaces: true });
    assert.deepEqual(vertices(batched.object), vertices(original.object));
    before += original.object.children.length;
    after += batched.object.children.length;
    if (batched.object.children.some((draw) => draw.geometry.hasAttribute("atlasRegion"))) {
      for (const draw of batched.object.children)
        for (const attribute of Object.values(draw.geometry.attributes))
          assert.notEqual(attribute.array.buffer, source, "a remaining view pins the full input buffer");
    }
    assert.equal(batched.animated, original.animated);
    batched.updateTime(3.5);
    for (const draw of batched.object.children) {
      assert.equal(draw.material.uniforms.time.value, 3.5);
      let disposed = 0;
      draw.geometry.addEventListener("dispose", () => disposed++);
      draw.material.addEventListener("dispose", () => disposed++);
      draw.userData.disposed = () => disposed;
    }
    batched.dispose();
    for (const draw of batched.object.children) assert.equal(draw.userData.disposed(), 2);
    original.dispose();
  }
  assert(after < before);
  atlas.dispose();
});

test("only compatible opaque and cutout states merge; transparent batches retain sorting boundaries", () => {
  const surface = { texture: "stone", region: [0, 0, 0.25, 0.25], mode: "opaque", doubleSided: false, unlit: false, opacity: 1, frames: 1, frameTime: 0 };
  const first = { surface }, second = { surface: { ...surface, texture: "dirt", region: [0.25, 0, 0.25, 0.25] } };
  const differences = [{ mode: "cutout" }, { doubleSided: true }, { unlit: true }, { opacity: 0.5 }, { frames: 16 }, { frameTime: 2 }, { mode: "blend" }, { mode: "blend" }];
  const batches = [first, second, ...differences.map((value) => ({ surface: { ...surface, ...value } }))];
  const groups = opaqueBatchGroups(batches);
  assert.deepEqual(groups[0], [first, second]);
  assert(groups.slice(1).every((group) => group.length === 1));
  assert.equal(groups.length, batches.length - 1);
});
