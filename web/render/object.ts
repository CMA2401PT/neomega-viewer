import type * as THREE from "three";

/** 批次和特效使用相同的生命周期，宿主无需为它们拼造整帧数据。 */
export interface RenderObject {
  object: THREE.Group;
  bounds: THREE.Box3;
  readonly animated: boolean;
  /** 由宿主在每次绘制前传入时间，单位为秒。 */
  updateTime(seconds: number): void;
  /** Dispose this object's buffers/materials. The shared atlas remains owned by the caller. */
  dispose(): void;
}
