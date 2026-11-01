export type Vec3 = [number, number, number];
export type AlphaMode = "opaque" | "cutout" | "blend";
/** 子区块以整数坐标定位，场景按每轴 16 个方块放置几何。 */
export interface SectionPos {
  x: number;
  y: number;
  z: number;
}
export interface Surface {
  texture: string;
  region: [number, number, number, number];
  mode: AlphaMode;
  doubleSided: boolean;
  unlit: boolean;
  opacity: number;
  frames: number;
  frameTime: number;
}
export type FloatAttribute = number[] | Float32Array;
export type IndexAttribute = number[] | Uint32Array;
export interface Batch {
  surface: Surface;
  positions: FloatAttribute;
  normals: FloatAttribute;
  shadeNormals: FloatAttribute;
  uvs: FloatAttribute;
  colors: FloatAttribute;
  indices: IndexAttribute;
}
export interface MeshData {
  batches: Batch[];
  bounds: { min: Vec3; max: Vec3 };
  quads: number;
}

export interface EffectMessage {
  pos: Vec3;
  effect: string;
}
export interface BlockActor {
  pos: Vec3;
  mesh: MeshData;
}
export interface FrameData {
  mesh: MeshData;
  effects: EffectMessage[];
  actors: BlockActor[];
}
