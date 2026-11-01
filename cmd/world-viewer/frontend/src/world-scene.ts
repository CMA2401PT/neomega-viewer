import * as THREE from "three";
import {
  createFrameObject,
  SurfaceMaterials,
  type FrameData,
  type FrameObject,
  type EffectRegistry,
} from "../../../../web/render/index.ts";
import { mergedGeometry, opaqueBatchGroups } from "../../../../web/render/batching.ts";
import type { SectionKey } from "./stream.ts";

// Small spatial batches retain independent section visibility and lifetime.
export const REGION_SECTIONS = 4;
const REGION_SIZE = REGION_SECTIONS * 16;
const REGION_CAPACITY = REGION_SECTIONS ** 3;
const capacity = (count: number): number => 2 ** Math.ceil(Math.log2(Math.max(256, count)));

interface Draw {
  geometry: THREE.BufferGeometry;
  material: THREE.ShaderMaterial;
}
export interface WorldSection {
  key: SectionKey;
  bounds: THREE.Box3;
  /** Temporary geometry is released as soon as the section enters a batch. */
  opaque: Draw[];
  frame?: FrameObject;
  /** Active geometry estimate; excludes spare capacity in shared batch buffers. */
  bytes: number;
  unknown: number;
  dispose(): void;
}
interface Instance {
  batch: RegionBatch;
  geometryID: number;
  instanceID: number;
  vertices: number;
  indices: number;
}
interface Entry {
  section: WorldSection;
  instances: Instance[];
  visible: boolean;
}
interface Region {
  id: string;
  group: THREE.Group;
  bounds: THREE.Box3;
  entries: Map<WorldSection, Entry>;
  batches: Map<THREE.ShaderMaterial, RegionBatch>;
  dirtyBounds: boolean;
}

/** 0: outside, 1: intersects, 2: fully inside. No per-test object allocation. */
export function classifyBox(frustum: THREE.Frustum, box: THREE.Box3): 0 | 1 | 2 {
  let inside = true;
  for (const { normal: n, constant } of frustum.planes) {
    const far = constant
      + n.x * (n.x >= 0 ? box.max.x : box.min.x)
      + n.y * (n.y >= 0 ? box.max.y : box.min.y)
      + n.z * (n.z >= 0 ? box.max.z : box.min.z);
    if (far < 0) return 0;
    const near = constant
      + n.x * (n.x >= 0 ? box.min.x : box.max.x)
      + n.y * (n.y >= 0 ? box.min.y : box.max.y)
      + n.z * (n.z >= 0 ? box.min.z : box.max.z);
    if (near < 0) inside = false;
  }
  return inside ? 2 : 1;
}

/** Amortized buffer growth; deleted sections are compacted before growing again. */
class RegionBatch {
  readonly mesh: THREE.BatchedMesh;
  private vertices = 0;
  private indices = 0;
  private vertexCapacity: number;
  private indexCapacity: number;
  private fragmented = false;

  constructor(draw: Draw) {
    this.vertexCapacity = capacity(draw.geometry.getAttribute("position").count);
    this.indexCapacity = capacity(draw.geometry.index!.count);
    this.mesh = new THREE.BatchedMesh(
      REGION_CAPACITY,
      this.vertexCapacity,
      this.indexCapacity,
      draw.material,
    );
    // WorldScene handles the region and section hierarchy once for every material.
    this.mesh.frustumCulled = false;
    this.mesh.perObjectFrustumCulled = false;
    this.mesh.sortObjects = false;
    // Used only for render-list sorting, not visibility (which uses actual bounds).
    this.mesh.boundingSphere = new THREE.Sphere(
      new THREE.Vector3().setScalar(REGION_SIZE / 2),
      Math.sqrt(3) * REGION_SIZE / 2,
    );
  }

  add(draw: Draw): Instance {
    const vertices = draw.geometry.getAttribute("position").count;
    const indices = draw.geometry.index!.count;
    if (this.mesh.unusedVertexCount < vertices || this.mesh.unusedIndexCount < indices) {
      if (this.fragmented) {
        this.mesh.optimize();
        this.fragmented = false;
      }
      if (this.mesh.unusedVertexCount < vertices || this.mesh.unusedIndexCount < indices) {
        this.vertexCapacity = Math.max(this.vertexCapacity, capacity(this.vertices + vertices));
        this.indexCapacity = Math.max(this.indexCapacity, capacity(this.indices + indices));
        this.mesh.setGeometrySize(this.vertexCapacity, this.indexCapacity);
      }
    }
    const geometryID = this.mesh.addGeometry(draw.geometry);
    const instanceID = this.mesh.addInstance(geometryID);
    this.vertices += vertices;
    this.indices += indices;
    return { batch: this, geometryID, instanceID, vertices, indices };
  }

  remove(instance: Instance): void {
    this.mesh.deleteGeometry(instance.geometryID);
    this.vertices -= instance.vertices;
    this.indices -= instance.indices;
    this.fragmented = true;
  }

  trim(): void {
    // Shrink only after substantial unloading, once per frame rather than per section.
    if (!this.fragmented || this.vertices >= this.vertexCapacity / 4) return;
    this.mesh.optimize();
    this.fragmented = false;
    this.vertexCapacity = capacity(this.vertices);
    this.indexCapacity = capacity(this.indices);
    this.mesh.setGeometrySize(this.vertexCapacity, this.indexCapacity);
  }
}

/** Owns world materials, regional GPU batches and the two-level visibility index. */
export class WorldScene {
  readonly materials: SurfaceMaterials;
  readonly visibility = { regionsTested: 0, sectionsTested: 0, visibleSections: 0 };
  private readonly regions = new Map<string, Region>();
  private readonly membership = new Map<WorldSection, Region>();
  private readonly frustum = new THREE.Frustum();
  private readonly projectionView = new THREE.Matrix4();
  private readonly scene: THREE.Scene;
  private readonly atlas: THREE.Texture;
  private readonly effects: EffectRegistry;
  private wireframe = false;

  constructor(scene: THREE.Scene, atlas: THREE.Texture, effects: EffectRegistry) {
    this.scene = scene;
    this.atlas = atlas;
    this.effects = effects;
    this.materials = new SurfaceMaterials(atlas);
  }

  prepare(key: SectionKey, data?: FrameData, unknown = 0): WorldSection {
    const section: WorldSection = {
      key,
      bounds: new THREE.Box3(),
      opaque: [],
      bytes: 512,
      unknown,
      dispose: () => {
        this.remove(section);
        for (const draw of section.opaque) draw.geometry.dispose();
        section.opaque.length = 0;
        section.frame?.dispose();
      },
    };
    if (!data) return section;
    const meshes = [data.mesh, ...data.actors.map((actor) => actor.mesh)];
    for (const mesh of meshes) {
      if (!mesh.quads) continue;
      section.bounds.union(new THREE.Box3(
        new THREE.Vector3(...mesh.bounds.min),
        new THREE.Vector3(...mesh.bounds.max),
      ));
    }
    const batches = meshes.flatMap((mesh) => mesh.batches);
    for (const group of opaqueBatchGroups(batches.filter((batch) => batch.surface.mode !== "blend"))) {
      section.opaque.push({
        geometry: mergedGeometry(group),
        material: this.materials.get(group[0].surface, true),
      });
    }
    const transparent = batches.filter((batch) => batch.surface.mode === "blend");
    if (transparent.length || data.effects.length) {
      section.frame = createFrameObject({
        mesh: {
          batches: transparent,
          quads: transparent.reduce((sum, batch) => sum + batch.indices.length / 6, 0),
          bounds: { min: section.bounds.min.toArray(), max: section.bounds.max.toArray() },
        },
        actors: [],
        effects: data.effects,
      }, this.atlas, this.effects, {
        sortTransparentQuads: false,
        materials: this.materials,
        copyBuffers: true,
      });
      section.bounds.union(section.frame.bounds);
      this.applyWireframe(section.frame.object);
    }
    const geometries = section.opaque.map((draw) => draw.geometry);
    section.frame?.object.traverse((object) => {
      if (object instanceof THREE.Mesh) geometries.push(object.geometry);
    });
    for (const geometry of geometries) {
      section.bytes += 512 + 2 * (
        Object.values(geometry.attributes).reduce((sum, attr) => sum + attr.array.byteLength, 0)
        + (geometry.index?.array.byteLength ?? 0)
      );
    }
    section.bounds.translate(new THREE.Vector3(key.x * 16, key.y * 16, key.z * 16));
    return section;
  }

  add(section: WorldSection): void {
    if (section.bounds.isEmpty()) return;
    const { key } = section;
    const origin = new THREE.Vector3(
      Math.floor(key.x / REGION_SECTIONS) * REGION_SIZE,
      Math.floor(key.y / REGION_SECTIONS) * REGION_SIZE,
      Math.floor(key.z / REGION_SECTIONS) * REGION_SIZE,
    );
    const id = origin.toArray().join(",");
    let region = this.regions.get(id);
    if (!region) {
      const group = new THREE.Group();
      group.position.copy(origin);
      this.scene.add(group);
      group.updateMatrixWorld(true);
      region = { id, group, bounds: new THREE.Box3(), entries: new Map(), batches: new Map(), dirtyBounds: false };
      this.regions.set(id, region);
    }
    const offset = new THREE.Vector3(key.x * 16, key.y * 16, key.z * 16).sub(origin);
    const entry: Entry = { section, instances: [], visible: true };
    for (const draw of section.opaque) {
      let batch = region.batches.get(draw.material);
      if (!batch) {
        batch = new RegionBatch(draw);
        region.batches.set(draw.material, batch);
        region.group.add(batch.mesh);
        batch.mesh.updateMatrixWorld(true);
      }
      // Section placement never changes. Bake region-local positions once instead
      // of fetching an instance matrix for every vertex on every frame.
      draw.geometry.translate(offset.x, offset.y, offset.z);
      entry.instances.push(batch.add(draw));
      draw.geometry.dispose();
    }
    section.opaque.length = 0;
    if (section.frame) {
      // The control can change while a prepared section is awaiting attachment.
      this.applyWireframe(section.frame.object);
      section.frame.object.position.copy(offset);
      region.group.add(section.frame.object);
      section.frame.object.updateMatrixWorld(true);
    }
    region.bounds.union(section.bounds);
    region.entries.set(section, entry);
    this.membership.set(section, region);
  }

  private remove(section: WorldSection): void {
    const region = this.membership.get(section);
    if (!region) return;
    const entry = region.entries.get(section)!;
    for (const instance of entry.instances) {
      const batch = instance.batch;
      batch.remove(instance);
      if (!batch.mesh.instanceCount) {
        region.group.remove(batch.mesh);
        batch.mesh.dispose();
        region.batches.delete(batch.mesh.material as THREE.ShaderMaterial);
      }
    }
    section.frame?.object.removeFromParent();
    region.entries.delete(section);
    this.membership.delete(section);
    if (!region.entries.size) {
      region.group.removeFromParent();
      this.regions.delete(region.id);
    } else {
      region.dirtyBounds = true;
    }
  }

  update(camera: THREE.PerspectiveCamera, seconds: number): void {
    this.materials.time.value = seconds;
    camera.updateMatrixWorld();
    this.projectionView.multiplyMatrices(camera.projectionMatrix, camera.matrixWorldInverse);
    this.frustum.setFromProjectionMatrix(this.projectionView);
    this.visibility.regionsTested = this.visibility.sectionsTested = this.visibility.visibleSections = 0;
    for (const region of this.regions.values()) {
      if (region.dirtyBounds) {
        region.bounds.makeEmpty();
        for (const { section } of region.entries.values()) region.bounds.union(section.bounds);
        for (const batch of region.batches.values()) batch.trim();
        region.dirtyBounds = false;
      }
      this.visibility.regionsTested++;
      const classification = classifyBox(this.frustum, region.bounds);
      region.group.visible = classification !== 0;
      if (!classification) continue;
      for (const entry of region.entries.values()) {
        const { section } = entry;
        if (classification !== 2) this.visibility.sectionsTested++;
        const visible = classification === 2 || this.frustum.intersectsBox(section.bounds);
        if (visible !== entry.visible) {
          for (const { batch, instanceID } of entry.instances) batch.mesh.setVisibleAt(instanceID, visible);
          entry.visible = visible;
        }
        if (section.frame) section.frame.object.visible = visible;
        if (!visible) continue;
        this.visibility.visibleSections++;
        if (section.frame?.animated) {
          section.frame.updateTime(seconds);
          section.frame.object.updateMatrixWorld();
        }
      }
    }
  }

  private applyWireframe(object: THREE.Object3D): void {
    object.traverse((draw) => {
      if (!(draw instanceof THREE.Mesh)) return;
      const materials = Array.isArray(draw.material) ? draw.material : [draw.material];
      for (const material of materials) {
        if (material instanceof THREE.ShaderMaterial || material instanceof THREE.MeshBasicMaterial)
          material.wireframe = this.wireframe;
      }
    });
  }

  setWireframe(value: boolean): void {
    this.wireframe = value;
    this.materials.setWireframe(value);
    for (const region of this.regions.values()) {
      for (const entry of region.entries.values()) {
        if (entry.section.frame) this.applyWireframe(entry.section.frame.object);
        // BatchedMesh caches draw ranges. A wireframe toggle changes their index layout.
        for (const { batch, instanceID } of entry.instances) {
          batch.mesh.setVisibleAt(instanceID, !entry.visible);
          batch.mesh.setVisibleAt(instanceID, entry.visible);
        }
      }
    }
  }

  dispose(): void {
    for (const section of this.membership.keys()) section.dispose();
    this.materials.dispose();
  }
}
