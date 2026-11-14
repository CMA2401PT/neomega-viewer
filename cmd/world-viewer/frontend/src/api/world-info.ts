export interface WorldInfo {
  revision: number;
  canOpen?: boolean;
  loaded: boolean;
  minimap?: boolean;
  minimapBounds?: [number, number];
  name: string;
  path?: string;
  kind?: string;
  dimension?: string;
  conversionSeconds?: number;
  reusedStructure?: boolean;
  position: [number, number, number];
  resourceVersion: string;
  version: string;
  /** 存在时表示默认场景。 */
  backdrop?: BackdropInfo;
}
export interface BackdropInfo {
  look: [number, number, number];
  fov: number;
  center: [number, number, number];
  radius: number;
  seaLevel: number;
  seaEnd: number;
}
