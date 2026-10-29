import * as THREE from "three";

/** A small transparent XYZ axis gizmo rendered on its own canvas. It mirrors
 * the main camera orientation so the viewer always knows which way is up. */
export class AxisGizmo {
  private readonly renderer: THREE.WebGLRenderer;
  private readonly scene = new THREE.Scene();
  private readonly camera = new THREE.OrthographicCamera(-1.6, 1.6, 1.6, -1.6, 0.1, 10);
  private readonly axes = new THREE.Group();
  private readonly tmp = new THREE.Quaternion();
  private scheduled = false;

  constructor(canvas: HTMLCanvasElement, size = 72) {
    this.renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: true });
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    this.renderer.setSize(size, size, false);
    this.camera.position.set(0, 0, 5);
    const colors = { x: 0xff6b7a, y: 0x7ce08a, z: 0x6aa8ff } as const;
    const dirs = {
      x: new THREE.Vector3(1, 0, 0),
      y: new THREE.Vector3(0, 1, 0),
      z: new THREE.Vector3(0, 0, 1),
    } as const;
    for (const key of ["x", "y", "z"] as const) {
      const dir = dirs[key];
      const line = new THREE.Line(
        new THREE.BufferGeometry().setFromPoints([new THREE.Vector3(), dir.clone()]),
        new THREE.LineBasicMaterial({ color: colors[key], transparent: true, opacity: 0.85 }),
      );
      const tip = new THREE.Mesh(
        new THREE.SphereGeometry(0.16, 16, 16),
        new THREE.MeshBasicMaterial({ color: colors[key] }),
      );
      tip.position.copy(dir);
      this.axes.add(line, tip);
    }
    this.scene.add(this.axes);
  }

  /** Point the gizmo opposite the camera so world axes read correctly. */
  update(cameraQuaternion: THREE.Quaternion): void {
    this.axes.quaternion.copy(this.tmp.copy(cameraQuaternion).invert());
    if (this.scheduled) return;
    this.scheduled = true;
    requestAnimationFrame(() => {
      this.scheduled = false;
      this.renderer.render(this.scene, this.camera);
    });
  }

  dispose(): void {
    this.renderer.dispose();
  }
}
