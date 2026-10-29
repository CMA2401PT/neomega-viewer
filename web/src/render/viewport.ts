import * as THREE from "three";
import { OrbitControls } from "three/addons/controls/OrbitControls.js";
import { createMeshObject, type MeshObject } from "./mesh";
import type { MeshData } from "./types";

/** A geometry viewport. It does not parse blocks, load a world or start workers. */
export class MeshViewport {
  private readonly renderer = new THREE.WebGLRenderer({
    antialias: true,
    alpha: false,
  });
  private readonly scene = new THREE.Scene();
  private camera: THREE.PerspectiveCamera | THREE.OrthographicCamera =
    new THREE.PerspectiveCamera(38, 1, 0.01, 100);
  private controls: OrbitControls;
  private readonly grid: THREE.GridHelper;
  private readonly frame = new THREE.Box3Helper(
    new THREE.Box3(new THREE.Vector3(), new THREE.Vector3(1, 1, 1)),
    0x9caaa9,
  );
  private readonly resize: ResizeObserver;
  private current?: MeshObject;
  private bounds = new THREE.Box3(
    new THREE.Vector3(),
    new THREE.Vector3(1, 1, 1),
  );
  private scheduled = false;
  private disposed = false;

  constructor(
    private readonly host: HTMLElement,
    private readonly atlas: THREE.Texture,
    options: { gridSize?: number; label?: string } = {},
  ) {
    this.grid = new THREE.GridHelper(
      options.gridSize ?? 8,
      32,
      0xb8c3c5,
      0xdde4e4,
    );
    this.scene.background = new THREE.Color(0xf0f4f3);
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    this.renderer.domElement.setAttribute(
      "aria-label",
      options.label ?? "静态几何三维预览，可拖动旋转和滚轮缩放",
    );
    host.append(this.renderer.domElement);
    this.controls = this.makeControls();
    this.frame.visible = false;
    this.scene.add(this.grid, this.frame);
    this.resize = new ResizeObserver(() => this.resizeCanvas());
    this.resize.observe(host);
    this.reset();
  }

  private makeControls(): OrbitControls {
    const controls = new OrbitControls(this.camera, this.renderer.domElement);
    controls.enableDamping = false;
    controls.minDistance = 0.2;
    controls.maxDistance = 50;
    controls.addEventListener("change", () => this.render());
    return controls;
  }

  set(data: MeshData, options: { resetView?: boolean } = {}): void {
    if (this.current) {
      this.scene.remove(this.current.object);
      this.current.dispose();
    }
    this.current = createMeshObject(data, this.atlas);
    this.scene.add(this.current.object);
    this.bounds =
      data.quads > 0
        ? new THREE.Box3(
            new THREE.Vector3(...data.bounds.min),
            new THREE.Vector3(...data.bounds.max),
          )
        : new THREE.Box3(new THREE.Vector3(), new THREE.Vector3(1, 1, 1));
    this.frame.box.copy(this.bounds);
    this.grid.position.set(
      this.bounds.getCenter(new THREE.Vector3()).x,
      this.bounds.min.y - 0.005,
      this.bounds.getCenter(new THREE.Vector3()).z,
    );
    if (options.resetView !== false) this.reset();
    else this.render();
  }

  reset(): void {
    const center = this.bounds.getCenter(new THREE.Vector3());
    const size = Math.max(1, this.bounds.getSize(new THREE.Vector3()).length());
    this.controls.maxDistance = size * 10;
    this.camera.far = Math.max(100, size * 20);
    this.controls.target.copy(center);
    this.camera.position
      .copy(center)
      .add(new THREE.Vector3(1.5, 1.05, 1.8).multiplyScalar(size));
    if (this.camera instanceof THREE.OrthographicCamera) this.camera.zoom = 1;
    this.camera.lookAt(center);
    this.resizeCanvas();
    this.controls.update();
    this.render();
  }

  projection(orthographic: boolean): void {
    this.controls.dispose();
    this.camera = orthographic
      ? new THREE.OrthographicCamera(-2, 2, 2, -2, 0.01, 100)
      : new THREE.PerspectiveCamera(38, 1, 0.01, 100);
    this.controls = this.makeControls();
    this.reset();
  }

  showWireframe(visible: boolean): void {
    this.current?.object.traverse((object) => {
      if (
        object instanceof THREE.Mesh &&
        object.material instanceof THREE.ShaderMaterial
      ) {
        object.material.wireframe = visible;
      }
    });
    this.render();
  }

  showFrame(visible: boolean): void {
    this.frame.visible = visible;
    this.render();
  }
  showGrid(visible: boolean): void {
    this.grid.visible = visible;
    this.render();
  }

  private resizeCanvas(): void {
    const width = this.host.clientWidth,
      height = this.host.clientHeight;
    if (width === 0 || height === 0) return;
    this.renderer.setSize(width, height, false);
    if (this.camera instanceof THREE.PerspectiveCamera)
      this.camera.aspect = width / height;
    else {
      const size =
        Math.max(1, this.bounds.getSize(new THREE.Vector3()).length()) * 1.15;
      this.camera.left = (-size * width) / height;
      this.camera.right = (size * width) / height;
      this.camera.top = size;
      this.camera.bottom = -size;
    }
    this.camera.updateProjectionMatrix();
    this.render();
  }

  private render(): void {
    if (this.scheduled || this.disposed) return;
    this.scheduled = true;
    requestAnimationFrame(() => {
      this.scheduled = false;
      if (!this.disposed) this.renderer.render(this.scene, this.camera);
    });
  }

  dispose(): void {
    this.disposed = true;
    this.resize.disconnect();
    this.controls.dispose();
    this.current?.dispose();
    this.grid.geometry.dispose();
    (this.grid.material as THREE.Material).dispose();
    this.frame.geometry.dispose();
    (this.frame.material as THREE.Material).dispose();
    this.renderer.dispose();
    this.renderer.domElement.remove();
  }
}

// Compatibility name for callers of the original single-block viewport.
export { MeshViewport as BlockViewport };
