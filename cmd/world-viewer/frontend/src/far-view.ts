import type * as THREE from "three";
import { BrickmapCoverage, type CoveredSection } from "../../../../../../neomega-viewer-core/viewer/brickmap/web/coverage.ts";
import type { BrickmapWindow } from "../../../../../../neomega-viewer-core/viewer/brickmap/web/gpu.ts";
import type { BrickmapWorld } from "./brickmap-world.ts";
import { WINDOW_SIZE } from "./map-regions.ts";

export const FAR_FOG: [number, number] = [WINDOW_SIZE / 3, WINDOW_SIZE / 2];

export class FarView {
  private coverage?: BrickmapCoverage;
  private window?: BrickmapWindow;
  private revision = -1;
  private origin = "";

  constructor(private readonly renderer: THREE.WebGLRenderer, private readonly terrain: BrickmapWorld) {}

  draw(camera: THREE.PerspectiveCamera, revision: number, sections: () => Iterable<CoveredSection>): boolean {
    // 远景先写颜色和透视深度，精细场景随后沿用同一深度缓冲。
    const gpu = this.terrain.far.gpu;
    if (!gpu) { this.dispose(); return false; }
    const gl = this.renderer.getContext() as WebGL2RenderingContext;
    if (this.window !== gpu.window) {
      this.dispose();
      this.window = gpu.window;
      this.coverage = new BrickmapCoverage(gl, gpu.window.size);
    }
    const origin = gpu.window.origin.join(",");
    if (revision !== this.revision || origin !== this.origin) {
      this.coverage!.update(gpu.window.origin, sections());
      this.revision = revision;
      this.origin = origin;
    }
    const e = camera.matrixWorld.elements;
    gpu.draw.drawPerspective({
      position: [camera.position.x, camera.position.y, camera.position.z],
      right: [e[0], e[1], e[2]],
      up: [e[4], e[5], e[6]],
      direction: [-e[8], -e[9], -e[10]],
      verticalFov: camera.fov,
      near: camera.near,
      far: camera.far,
    }, {
      coverage: this.coverage,
      fog: FAR_FOG,
      background: [0xd9 / 255, 0xde / 255, 0xe3 / 255],
      openBoundary: true,
    });
    this.renderer.resetState();
    return true;
  }

  dispose(): void {
    this.coverage?.dispose();
    this.coverage = undefined;
    this.window = undefined;
    this.revision = -1;
    this.origin = "";
  }
}
