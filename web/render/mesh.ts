import * as THREE from "three";
import { surfaceMaterial, type SurfaceMaterials } from "./materials.ts";
import { mergedGeometry, opaqueBatchGroups } from "./batching.ts";
import type { RenderObject } from "./object.ts";
import type {
  Batch,
  MeshData,
  FloatAttribute,
  IndexAttribute,
} from "./types.ts";

// Typed binary views are retained directly. JSON arrays are converted once.
function floats(
  values: FloatAttribute,
  start: number,
  count: number,
  copy = false,
): Float32Array {
  return values instanceof Float32Array
    ? copy
      ? values.slice(start, start + count)
      : values.subarray(start, start + count)
    : new Float32Array(values.slice(start, start + count));
}

function buffers(
  batch: Batch,
  start: number,
  count: number,
  indices: IndexAttribute,
  copy = false,
): THREE.BufferGeometry {
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute(
    "position",
    new THREE.BufferAttribute(floats(batch.positions, start * 3, count * 3, copy), 3),
  );
  geometry.setAttribute(
    "normal",
    new THREE.BufferAttribute(floats(batch.normals, start * 3, count * 3, copy), 3),
  );
  geometry.setAttribute(
    "shadeNormal",
    new THREE.BufferAttribute(
      floats(batch.shadeNormals, start * 3, count * 3, copy),
      3,
    ),
  );
  geometry.setAttribute(
    "color",
    new THREE.BufferAttribute(floats(batch.colors, start * 3, count * 3, copy), 3),
  );
  geometry.setAttribute(
    "uv",
    new THREE.BufferAttribute(floats(batch.uvs, start * 2, count * 2, copy), 2),
  );
  geometry.setIndex(
    new THREE.BufferAttribute(
      indices instanceof Uint32Array
        ? copy ? indices.slice() : indices
        : new Uint32Array(indices),
      1,
    ),
  );
  geometry.computeBoundingSphere();
  return geometry;
}

export type MeshObject = RenderObject;

export interface MeshOptions {
  sortTransparentQuads?: boolean;
  /** Merge compatible opaque/cutout surfaces using per-vertex atlas regions. */
  batchOpaqueSurfaces?: boolean;
  /** Shared materials are owned by the caller and outlive this mesh. */
  materials?: SurfaceMaterials;
  /** Detach small retained batches from a larger decoded frame buffer. */
  copyBuffers?: boolean;
}

/** Draw already baked geometry. This module never receives blockstates/models. */
export function createMeshObject(
  data: MeshData,
  atlas: THREE.Texture,
  options: MeshOptions = {},
): MeshObject {
  return createBatchObject(data.batches, atlas, options);
}

/** 子区块可直接绘制筛选后的批次，边界从实际几何计算。 */
export function createBatchObject(
  batches: readonly Batch[],
  atlas: THREE.Texture,
  options: MeshOptions = {},
): RenderObject {
  const object = new THREE.Group();
  const bounds = new THREE.Box3();
  const geometries: THREE.BufferGeometry[] = [];
  const materials: THREE.Material[] = [];
  // 所有批次共享时钟，透明面拆分后仍保持同一动画相位。
  const time = options.materials?.time ?? { value: 0 };
  const animated = batches.some((batch) => batch.surface.frames > 1);
  const groups = options.batchOpaqueSurfaces
    ? opaqueBatchGroups(batches)
    : batches.map((batch) => [batch]);
  // Once merged attributes replace most source views, copy the remaining views
  // too: a small water batch should not retain the entire original NVFR buffer.
  const copySingleBatches = options.copyBuffers || groups.some((batches) => batches.length > 1);
  for (const batches of groups) {
    const batch = batches[0];
    const material = options.materials
      ? options.materials.get(batch.surface, batches.length > 1)
      : surfaceMaterial(batch.surface, atlas, time, batches.length > 1);
    if (!options.materials) materials.push(material);
    const count = batch.positions.length / 3;
    if (batches.length > 1) {
      const geometry = mergedGeometry(batches);
      geometries.push(geometry);
      object.add(new THREE.Mesh(geometry, material));
    } else if (batch.surface.mode === "blend" && options.sortTransparentQuads !== false) {
      // Three sorts transparent objects, not triangles inside one buffer.
      // Separate static quads make the near/far glass surfaces sortable.
      for (let start = 0; start < count; start += 4) {
        const geometry = buffers(batch, start, 4, [0, 1, 2, 0, 2, 3], copySingleBatches);
        geometries.push(geometry);
        object.add(new THREE.Mesh(geometry, material));
      }
    } else {
      const geometry = buffers(batch, 0, count, batch.indices, copySingleBatches);
      geometries.push(geometry);
      object.add(new THREE.Mesh(geometry, material));
    }
  }
  for (const geometry of geometries) {
    geometry.computeBoundingBox();
    bounds.union(geometry.boundingBox!);
  }
  return {
    object,
    bounds,
    animated,
    updateTime(seconds: number) {
      time.value = seconds;
    },
    dispose() {
      geometries.forEach((g) => g.dispose());
      materials.forEach((m) => m.dispose());
    },
  };
}

export async function loadAtlas(url: string): Promise<THREE.Texture> {
  const texture = await new THREE.TextureLoader().loadAsync(url);
  texture.colorSpace = THREE.SRGBColorSpace;
  texture.flipY = false; // Go's UVs use PNG's top-left origin.
  texture.magFilter = THREE.NearestFilter;
  texture.minFilter = THREE.NearestFilter;
  texture.generateMipmaps = false;
  return texture;
}
