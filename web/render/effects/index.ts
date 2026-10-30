import type * as THREE from "three";
import type { EffectMessage } from "../types.ts";
import { createEnchantingTableBook } from "./book.ts";

export interface EffectObject {
  object: THREE.Object3D;
  /** 包含整个动画范围，换帧后相机取景保持稳定。 */
  bounds: THREE.Box3;
  /** 与网格纹理共用宿主时钟，单位为秒。 */
  update(time: number): void;
  dispose(): void;
}
export type EffectFactory = (message: EffectMessage) => EffectObject;
export type EffectRegistry = ReadonlyMap<string, EffectFactory>;

/** 工厂借用贴图，贴图由调用方加载和释放；可在返回的 Map 中注册其他特效。 */
export function createEffectRegistry(
  bookTexture: THREE.Texture,
): Map<string, EffectFactory> {
  return new Map([
    [
      "enchanting_table_book",
      (message) => createEnchantingTableBook(message, bookTexture),
    ],
  ]);
}
