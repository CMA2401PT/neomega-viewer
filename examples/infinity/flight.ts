import * as THREE from "three";

/** 拖动调整视线，WASD 只沿水平面移动，空格和 Shift 控制升降。 */
export class FlightControls {
  private readonly keys = new Set<string>();
  private readonly direction = new THREE.Vector3();
  private readonly rotation = new THREE.Euler(-0.35, 0, 0, "YXZ");
  private readonly camera: THREE.PerspectiveCamera;

  constructor(camera: THREE.PerspectiveCamera, canvas: HTMLCanvasElement) {
    this.camera = camera;
    camera.quaternion.setFromEuler(this.rotation);
    canvas.tabIndex = 0;
    const movementKeys = new Set(["KeyW", "KeyA", "KeyS", "KeyD", "Space", "ShiftLeft", "ShiftRight"]);
    canvas.addEventListener("keydown", event => {
      if (movementKeys.has(event.code)) {
        event.preventDefault();
        this.keys.add(event.code);
      }
    });
    canvas.addEventListener("keyup", event => this.keys.delete(event.code));
    canvas.addEventListener("blur", () => this.keys.clear());
    canvas.addEventListener("pointerdown", event => {
      canvas.focus();
      canvas.setPointerCapture(event.pointerId);
    });
    canvas.addEventListener("pointermove", event => {
      if (!canvas.hasPointerCapture(event.pointerId)) return;
      this.rotation.y -= event.movementX * 0.003;
      // 避免翻过头顶，水平移动方向始终由偏航角决定。
      this.rotation.x = THREE.MathUtils.clamp(
        this.rotation.x - event.movementY * 0.003,
        -1.55,
        1.55,
      );
      camera.quaternion.setFromEuler(this.rotation);
    });
    canvas.addEventListener("contextmenu", event => event.preventDefault());
    canvas.focus();
  }

  /** 使用秒数计算位移，斜向移动与直行保持相同速度。 */
  update(seconds: number): void {
    const forward = Number(this.keys.has("KeyW")) - Number(this.keys.has("KeyS"));
    const right = Number(this.keys.has("KeyD")) - Number(this.keys.has("KeyA"));
    const up = Number(this.keys.has("Space")) - Number(this.keys.has("ShiftLeft") || this.keys.has("ShiftRight"));
    const yaw = this.rotation.y;
    this.direction.set(
      right * Math.cos(yaw) - forward * Math.sin(yaw),
      up,
      -right * Math.sin(yaw) - forward * Math.cos(yaw),
    ).normalize();
    this.camera.position.addScaledVector(this.direction, seconds * 16);
  }
}
