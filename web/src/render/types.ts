export type Vec3 = [number, number, number];
export type AlphaMode = "opaque" | "cutout" | "blend";
export interface Surface {
  texture: string;
  region: [number, number, number, number];
  mode: AlphaMode;
  doubleSided: boolean;
  unlit: boolean;
  opacity: number;
}
export interface Batch {
  surface: Surface;
  positions: number[];
  normals: number[];
  shadeNormals: number[];
  uvs: number[];
  colors: number[];
  indices: number[];
}
export interface MeshData {
  batches: Batch[];
  bounds: { min: Vec3; max: Vec3 };
  quads: number;
}
