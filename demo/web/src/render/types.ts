export type Vec3 = [number, number, number];
export type AlphaMode = "opaque" | "cutout" | "blend";
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
