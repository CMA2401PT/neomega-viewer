import * as THREE from "three";

/** Loaded geometry buffers only: retained CPU storage plus an estimated GPU copy.
 * Include unused batch capacity; count shared arrays and GPU attributes once.
 * Textures, JS objects, render targets and transient loading data are not included. */
export function geometryMemoryBytes(root: THREE.Object3D): number {
  const cpuBuffers = new Set<ArrayBufferLike>();
  const gpuBuffers = new Set<THREE.BufferAttribute | THREE.InterleavedBuffer>();
  root.traverse((object) => {
    if (!(object instanceof THREE.Mesh)) return;
    const geometry: THREE.BufferGeometry = object.geometry;
    const attributes = Object.values(geometry.attributes);
    if (geometry.index) attributes.push(geometry.index);
    for (const attribute of attributes) {
      const buffer = attribute instanceof THREE.InterleavedBufferAttribute
        ? attribute.data
        : attribute;
      cpuBuffers.add(buffer.array.buffer);
      gpuBuffers.add(buffer);
    }
  });
  let bytes = 0;
  for (const buffer of cpuBuffers) bytes += buffer.byteLength;
  for (const buffer of gpuBuffers) bytes += buffer.array.byteLength;
  return bytes;
}

/** Average animation-frame cadence over at least one second. */
export class FrameRate {
  private start: number | undefined;
  private frames = 0;

  sample(now: number): number | undefined {
    if (this.start === undefined) {
      this.start = now;
      return;
    }
    this.frames++;
    const elapsed = now - this.start;
    if (elapsed < 1000) return;
    const fps = this.frames * 1000 / elapsed;
    this.start = now;
    this.frames = 0;
    return fps;
  }

  reset(): void {
    this.start = undefined;
    this.frames = 0;
  }
}
