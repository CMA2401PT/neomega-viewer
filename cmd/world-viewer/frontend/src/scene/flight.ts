import * as THREE from "three";

interface FlightOptions {
  paused(): boolean;
  interaction(): void;
  changed(): void;
}

export class FlightControls {
  // 输入状态只属于主相机，小地图展开时由宿主暂停并清空。
  readonly rotation = new THREE.Euler(-0.35, 0, 0, "YXZ");
  private readonly pressed = new Set<string>();
  private readonly events = new AbortController();
  private readonly forward = new THREE.Vector3();
  private readonly right = new THREE.Vector3();
  private dragging = false;
  private readonly camera: THREE.PerspectiveCamera;
  private readonly options: FlightOptions;

  constructor(
    camera: THREE.PerspectiveCamera,
    canvas: HTMLCanvasElement,
    options: FlightOptions,
  ) {
    this.camera = camera;
    this.options = options;
    camera.quaternion.setFromEuler(this.rotation);
    const signal = this.events.signal;
    canvas.addEventListener("pointerdown", (event) => {
      if (options.paused() || (event.button !== 0 && event.button !== 2)) return;
      canvas.focus();
      options.interaction();
      this.dragging = true;
      canvas.setPointerCapture(event.pointerId);
    }, { signal });
    canvas.addEventListener("pointermove", (event) => {
      if (!this.dragging || options.paused()) return;
      this.rotation.y -= event.movementX * 0.004;
      this.rotation.x = THREE.MathUtils.clamp(
        this.rotation.x - event.movementY * 0.004,
        -Math.PI / 2 + 0.01,
        Math.PI / 2 - 0.01,
      );
      camera.quaternion.setFromEuler(this.rotation);
    }, { signal });
    canvas.addEventListener("pointerup", () => {
      this.dragging = false;
    }, { signal });
    canvas.addEventListener("pointercancel", () => {
      this.dragging = false;
    }, { signal });
    canvas.addEventListener("contextmenu", (event) =>
      event.preventDefault(),
      { signal },
    );
    canvas.addEventListener(
      "wheel",
      (event) => {
        event.preventDefault();
        if (options.paused()) return;
        options.interaction();
        camera.getWorldDirection(this.forward);
        camera.position.addScaledVector(
          this.forward,
          -Math.sign(event.deltaY) * (event.shiftKey ? 32 : 8),
        );
        options.changed();
      },
      { passive: false, signal },
    );
    canvas.addEventListener("keydown", (event) => {
      if (options.paused()) return;
      if (
        [
          "KeyW",
          "KeyA",
          "KeyS",
          "KeyD",
          "Space",
          "ShiftLeft",
          "ShiftRight",
        ].includes(event.code)
      ) {
        event.preventDefault();
        options.interaction();
        this.pressed.add(event.code);
      }
    }, { signal });
    window.addEventListener("keyup", (event) => this.pressed.delete(event.code), { signal });
    canvas.addEventListener("blur", () => this.pressed.clear(), { signal });
    window.addEventListener("blur", () => {
      this.pressed.clear();
      this.dragging = false;
    }, { signal });
  }

  clamp(): void {
    this.camera.position.x = THREE.MathUtils.clamp(
      this.camera.position.x,
      -29_999_999,
      29_999_999,
    );
    this.camera.position.z = THREE.MathUtils.clamp(
      this.camera.position.z,
      -29_999_999,
      29_999_999,
    );
    this.camera.position.y = THREE.MathUtils.clamp(this.camera.position.y, -2048, 2047);
  }

  update(dt: number): void {
    // 水平移动只取偏航角，抬头和低头不会改变移动高度。
    if (!this.pressed.size || this.options.paused()) return;
    const speed = 24 * dt;
    const camera = this.camera;
    this.forward.set(-Math.sin(this.rotation.y), 0, -Math.cos(this.rotation.y));
    this.right.set(Math.cos(this.rotation.y), 0, -Math.sin(this.rotation.y));
    camera.position.addScaledVector(
      this.forward,
      (Number(this.pressed.has("KeyW")) - Number(this.pressed.has("KeyS"))) * speed,
    );
    camera.position.addScaledVector(
      this.right,
      (Number(this.pressed.has("KeyD")) - Number(this.pressed.has("KeyA"))) * speed,
    );
    camera.position.y +=
      (Number(this.pressed.has("Space")) -
        Number(this.pressed.has("ShiftLeft") || this.pressed.has("ShiftRight"))) * speed;
    this.clamp();
    this.options.changed();
  }

  reset(position: [number, number, number], look?: [number, number, number]): void {
    this.cancel();
    this.camera.position.set(...position);
    if (look) {
      this.camera.lookAt(...look);
      this.rotation.setFromQuaternion(this.camera.quaternion);
    } else {
      this.rotation.set(-0.35, 0, 0, "YXZ");
      this.camera.quaternion.setFromEuler(this.rotation);
    }
  }

  cancel(): void {
    this.pressed.clear();
    this.dragging = false;
  }

  dispose(): void {
    this.cancel();
    this.events.abort();
  }
}
