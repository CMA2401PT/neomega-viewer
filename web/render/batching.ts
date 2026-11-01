import * as THREE from "three";
import { surfaceStateKey } from "./materials.ts";
import type { Batch } from "./types.ts";

/** Atlas regions can vary within a draw; blending and other shader settings cannot. */
export function opaqueBatchGroups(batches: readonly Batch[]): Batch[][] {
  const groups = new Map<string, Batch[]>();
  const result: Batch[][] = [];
  for (const batch of batches) {
    const surface = batch.surface;
    // Preserve the existing transparent objects and their sorting granularity.
    if (surface.mode === "blend") {
      result.push([batch]);
      continue;
    }
    const key = surfaceStateKey(surface);
    let group = groups.get(key);
    if (!group) {
      group = [];
      groups.set(key, group);
      result.push(group);
    }
    group.push(batch);
  }
  return result;
}

/** Build GPU attributes from existing NVMS batches, without changing their faces or UVs. */
export function mergedGeometry(batches: Batch[]): THREE.BufferGeometry {
  const vertices = batches.reduce((sum, batch) => sum + batch.positions.length / 3, 0);
  const indexCount = batches.reduce((sum, batch) => sum + batch.indices.length, 0);
  const geometry = new THREE.BufferGeometry();
  // The shader uses shadeNormal; the separate geometric normal is not sampled.
  const attributes = [
    ["position", "positions", 3],
    ["shadeNormal", "shadeNormals", 3],
    ["color", "colors", 3],
    ["uv", "uvs", 2],
  ] as const;
  for (const [name, source, size] of attributes) {
    const values = new Float32Array(vertices * size);
    let offset = 0;
    for (const batch of batches) {
      values.set(batch[source], offset);
      offset += batch[source].length;
    }
    geometry.setAttribute(name, new THREE.BufferAttribute(values, size));
  }
  const regions = new Float32Array(vertices * 4);
  const indices = new Uint32Array(indexCount);
  let vertexOffset = 0, indexOffset = 0;
  for (const batch of batches) {
    const count = batch.positions.length / 3;
    for (let vertex = 0; vertex < count; vertex++)
      regions.set(batch.surface.region, (vertexOffset + vertex) * 4);
    for (const index of batch.indices) indices[indexOffset++] = vertexOffset + index;
    vertexOffset += count;
  }
  geometry.setAttribute("atlasRegion", new THREE.BufferAttribute(regions, 4));
  geometry.setIndex(new THREE.BufferAttribute(indices, 1));
  geometry.computeBoundingSphere();
  return geometry;
}
