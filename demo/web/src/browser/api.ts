import type { MeshData } from "../render";
export interface State {
  name: string;
  properties: Record<string, string>;
}
export interface Binding {
  id: number;
  java: State;
}
export interface Group {
  name: string;
  count: number;
  exampleId: number;
  javaNames: string[];
}
export interface Catalog {
  version: string;
  atlasUrl: string;
  groups: Group[];
}
export interface Packet {
  binding: Binding;
  renderer: { kind: string; family?: string };
  geometry: MeshData;
}
export async function getJSON<T>(
  path: string,
  options?: RequestInit,
): Promise<T> {
  const response = await fetch(path, options);
  if (!response.ok) {
    const text = await response.text();
    try {
      throw new Error((JSON.parse(text) as { error: string }).error);
    } catch (error) {
      if (error instanceof SyntaxError) throw new Error(text);
      throw error;
    }
  }
  return response.json() as Promise<T>;
}
