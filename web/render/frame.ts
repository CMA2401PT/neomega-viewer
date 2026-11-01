import * as THREE from "three";
import { createMeshObject, type MeshObject, type MeshOptions } from "./mesh.ts";
import type { EffectObject, EffectRegistry } from "./effects/index.ts";
import type { FrameData } from "./types.ts";

export interface FrameObject extends MeshObject {
  bounds: THREE.Box3;
}

/** 三段作为同一场景中的普通对象，共享深度缓冲和透明排序。 */
export function createFrameObject(
  data: FrameData,
  atlas: THREE.Texture,
  registry: EffectRegistry = new Map(),
  options: MeshOptions = {},
): FrameObject {
  const object = new THREE.Group();
  const bounds = new THREE.Box3();
  const meshes = [data.mesh, ...data.actors.map((actor) => actor.mesh)].map(
    (mesh) => {
      if (mesh.quads > 0) {
        bounds.union(new THREE.Box3(
          new THREE.Vector3(...mesh.bounds.min),
          new THREE.Vector3(...mesh.bounds.max),
        ));
      }
      return createMeshObject(mesh, atlas, options);
    },
  );
  const effects: EffectObject[] = [];
  for (const message of data.effects) {
    const factory = registry.get(message.effect);
    if (!factory) continue;
    const effect = factory(message);
    effects.push(effect);
    bounds.union(effect.bounds);
  }
  for (const draw of [...meshes, ...effects]) object.add(draw.object);
  if (bounds.isEmpty()) bounds.set(new THREE.Vector3(), new THREE.Vector3(1, 1, 1));
  return {
    object,
    bounds,
    animated: effects.length > 0 || meshes.some((mesh) => mesh.animated),
    updateTime(seconds: number) {
      meshes.forEach((mesh) => mesh.updateTime(seconds));
      effects.forEach((effect) => effect.update(seconds));
    },
    dispose() {
      meshes.forEach((mesh) => mesh.dispose());
      effects.forEach((effect) => effect.dispose());
    },
  };
}
