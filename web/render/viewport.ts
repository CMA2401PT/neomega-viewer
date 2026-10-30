import * as THREE from "three";
import { OrbitControls } from "three/addons/controls/OrbitControls.js";
import { createFrameObject, type FrameObject } from "./frame.ts";
import type { FrameData, MeshData } from "./types.ts";
import type { EffectRegistry } from "./effects/index.ts";

/** A geometry viewport. It does not parse blocks, load a world or start workers. */
export class MeshViewport {
  // Transparent canvas so the page's CSS backdrop shows through full-bleed.
  private readonly renderer = new THREE.WebGLRenderer({
    antialias: true,
    alpha: true,
  });
  private readonly scene = new THREE.Scene();
  private camera: THREE.PerspectiveCamera | THREE.OrthographicCamera =
    new THREE.PerspectiveCamera(38, 1, 0.01, 100);
  private controls: OrbitControls;
  private readonly grid: THREE.GridHelper;
  private readonly frame = new THREE.Box3Helper(
    new THREE.Box3(new THREE.Vector3(), new THREE.Vector3(1, 1, 1)),
    0x1246b4, // Bauhaus blue bounding outline, reads on the light stage.
  );
  private readonly resize: ResizeObserver;
  // Called every camera change so the host can drive an orientation gizmo.
  private onCamera?: (quaternion: THREE.Quaternion) => void;
  private current?: FrameObject;
  private readonly effectRegistry: EffectRegistry;
  private bounds = new THREE.Box3(
    new THREE.Vector3(),
    new THREE.Vector3(1, 1, 1),
  );
  private readonly host: HTMLElement;
  private readonly atlas: THREE.Texture;
  private scheduled = false;
  private disposed = false;

  constructor(
    host: HTMLElement,
    atlas: THREE.Texture,
    options: {
      gridSize?: number;
      label?: string;
      effects?: EffectRegistry;
    } = {},
  ) {
    this.host = host;
    this.atlas = atlas;
    this.effectRegistry = options.effects ?? new Map();
    const gridSpan = options.gridSize ?? 8;
    // Monochrome ground grid: a darker centre cross over hairline grey lines,
    // tuned to read on the light stage without competing with the geometry.
    this.grid = new THREE.GridHelper(
      gridSpan,
      gridSpan * 4,
      0x555555,
      0xb8b8b4,
    );
    const gridMaterial = this.grid.material as THREE.Material;
    gridMaterial.transparent = true;
    gridMaterial.opacity = 0.9;
    // Background stays null so the transparent canvas reveals the CSS scene.
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
    controls.enableDamping = true;
    controls.dampingFactor = 0.12;
    controls.minDistance = 0.2;
    controls.maxDistance = 50;
    controls.addEventListener("change", () => {
      this.onCamera?.(this.camera.quaternion);
      this.render();
    });
    return controls;
  }

  set(data: MeshData, options: { resetView?: boolean } = {}): void {
    this.setFrame({ mesh: data, actors: [], effects: [] }, options);
  }

  /** 一次替换三个通道；actor 顶点已带位置，未知特效名直接忽略。 */
  setFrame(data: FrameData, options: { resetView?: boolean } = {}): void {
    if (this.current) {
      this.scene.remove(this.current.object);
      this.current.dispose();
    }
    this.current = createFrameObject(data, this.atlas, this.effectRegistry);
    this.scene.add(this.current.object);
    this.bounds.copy(this.current.bounds);
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
    this.onCamera?.(this.camera.quaternion);
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
        (object.material instanceof THREE.ShaderMaterial ||
          object.material instanceof THREE.MeshBasicMaterial)
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
    requestAnimationFrame((timestamp) => {
      this.scheduled = false;
      if (this.disposed) return;
      // update() returns true while damping is still easing the camera; keep
      // scheduling frames until it settles so motion stays smooth.
      if (this.controls.update()) this.render();
      this.current?.updateTime(timestamp / 1000);
      this.renderer.render(this.scene, this.camera);
      // 相机静止后，特效和动画材质仍需持续重绘。
      if (this.current?.animated) this.render();
    });
  }

  /** Register a listener fired with the camera orientation on every change. */
  observeCamera(callback: (quaternion: THREE.Quaternion) => void): void {
    this.onCamera = callback;
    callback(this.camera.quaternion);
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
