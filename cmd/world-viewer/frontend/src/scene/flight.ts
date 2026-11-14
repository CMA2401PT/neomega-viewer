import * as THREE from "three";

interface FlightOptions {
  paused(): boolean;
  interaction(): void;
  changed(): void;
}

/** 相对相机朝向的水平移动，以及沿世界 Y 轴的升降。 */
export type Move = "forward" | "back" | "left" | "right" | "up" | "down";

const KEY_MOVES: Record<string, Move> = {
  KeyW: "forward",
  KeyS: "back",
  KeyA: "left",
  KeyD: "right",
  Space: "up",
  ShiftLeft: "down",
  ShiftRight: "down",
};

export class FlightControls {
  // 输入状态只属于主相机，小地图展开时由宿主暂停并清空。
  readonly rotation = new THREE.Euler(-0.35, 0, 0, "YXZ");
  // 键码或屏幕按钮 → 方向；同一方向可以被几个来源同时按住，松开一个不影响另一个。
  private readonly held = new Map<string, Move>();
  private readonly events = new AbortController();
  private readonly forward = new THREE.Vector3();
  private readonly right = new THREE.Vector3();
  // 只跟随第一根按下的手指或鼠标，另一根手指可以同时按住移动按钮。
  private drag?: { id: number; x: number; y: number };
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
      if (options.paused() || this.drag || (event.button !== 0 && event.button !== 2)) return;
      canvas.focus();
      options.interaction();
      this.drag = { id: event.pointerId, x: event.clientX, y: event.clientY };
      canvas.setPointerCapture(event.pointerId);
    }, { signal });
    canvas.addEventListener("pointermove", (event) => {
      const drag = this.drag;
      if (!drag || drag.id !== event.pointerId || options.paused()) return;
      this.rotation.y -= (event.clientX - drag.x) * 0.004;
      this.rotation.x = THREE.MathUtils.clamp(
        this.rotation.x - (event.clientY - drag.y) * 0.004,
        -Math.PI / 2 + 0.01,
        Math.PI / 2 - 0.01,
      );
      drag.x = event.clientX;
      drag.y = event.clientY;
      camera.quaternion.setFromEuler(this.rotation);
    }, { signal });
    const release = (event: PointerEvent): void => {
      if (this.drag?.id === event.pointerId) this.drag = undefined;
    };
    canvas.addEventListener("pointerup", release, { signal });
    canvas.addEventListener("pointercancel", release, { signal });
    canvas.addEventListener("contextmenu", (event) => event.preventDefault(), { signal });
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
      const move = KEY_MOVES[event.code];
      if (options.paused() || !move) return;
      event.preventDefault();
      options.interaction();
      this.held.set(event.code, move);
    }, { signal });
    window.addEventListener("keyup", (event) => this.held.delete(event.code), { signal });
    canvas.addEventListener("blur", () => {
      for (const code of Object.keys(KEY_MOVES)) this.held.delete(code);
    }, { signal });
    window.addEventListener("blur", () => this.cancel(), { signal });
  }

  /** 屏幕移动按钮按下和松开时调用，按住期间与对应的键盘按键效果相同。 */
  hold(move: Move, down: boolean): void {
    if (!down) {
      this.held.delete(`pad:${move}`);
      return;
    }
    if (this.options.paused()) return;
    this.options.interaction();
    this.held.set(`pad:${move}`, move);
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
    if (!this.held.size || this.options.paused()) return;
    const moves = new Set(this.held.values());
    const axis = (plus: Move, minus: Move): number => Number(moves.has(plus)) - Number(moves.has(minus));
    const speed = 24 * dt;
    const camera = this.camera;
    this.forward.set(-Math.sin(this.rotation.y), 0, -Math.cos(this.rotation.y));
    this.right.set(Math.cos(this.rotation.y), 0, -Math.sin(this.rotation.y));
    camera.position.addScaledVector(this.forward, axis("forward", "back") * speed);
    camera.position.addScaledVector(this.right, axis("right", "left") * speed);
    camera.position.y += axis("up", "down") * speed;
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
    this.held.clear();
    this.drag = undefined;
  }

  dispose(): void {
    this.cancel();
    this.events.abort();
  }
}
