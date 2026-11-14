import * as THREE from "three";
import type { WorldInfo } from "../api/world-info.ts";
import type { StreamStats } from "../scene/stream.ts";
import type { FlightControls } from "../scene/flight.ts";
import { Backdrop } from "./backdrop.ts";
import { createStage } from "./stage.ts";

interface IntroOptions {
  renderer: THREE.WebGLRenderer;
  camera: THREE.PerspectiveCamera;
  scene: THREE.Scene;
  controls: FlightControls;
  wireframe: HTMLInputElement;
  radius: HTMLInputElement;
  background: number;
  draw(): void;
}

export class Intro {
  // 默认场景的取景、漂移和点亮时机一起进入和退出，存档加载只提供进度。
  readonly stage = createStage();
  backdrop?: Backdrop;
  private radiusBefore?: string;
  private swayOrigin?: THREE.Vector3;
  private revealWanted = false;
  private stats?: StreamStats;
  private readonly events = new AbortController();

  constructor(private readonly options: IntroOptions) {
    for (const type of ["pointermove", "pointerdown"] as const) {
      window.addEventListener(type, () => {
        if (this.revealWanted) return;
        this.revealWanted = true;
        this.reveal();
      }, { signal: this.events.signal });
    }
  }

  show(info: WorldInfo): void {
    // 返回存档时恢复用户视距；场景方块仍使用相同的加载与绘制流程。
    const { renderer, camera, scene, controls, wireframe, radius, background, draw } = this.options;
    const atmosphere = info.backdrop;
    document.body.dataset.scene = atmosphere ? "backdrop" : "world";
    wireframe.checked = atmosphere !== undefined;
    this.backdrop?.dispose();
    this.backdrop = undefined;
    this.stats = undefined;
    if (atmosphere) {
      camera.position.set(...info.position);
      camera.fov = atmosphere.fov;
      camera.updateProjectionMatrix();
      this.backdrop = new Backdrop(renderer, camera, atmosphere, draw);
      scene.background = null;
      this.radiusBefore ??= radius.value;
      radius.value = "18";
    } else {
      camera.fov = 65;
      scene.background = new THREE.Color(background);
      if (this.radiusBefore !== undefined) radius.value = this.radiusBefore;
      this.radiusBefore = undefined;
    }
    this.stage.show(atmosphere);
    controls.reset(info.position, atmosphere?.look);
    this.swayOrigin = atmosphere ? camera.position.clone() : undefined;
  }

  loading(stats: StreamStats): void {
    this.stats = stats;
    this.reveal();
  }

  private reveal(): void {
    // 距离优先加载完成四分之一时，近处环与海已经可以点亮。
    if (!this.backdrop || !this.stats?.loaded) return;
    if (!this.stats.pending || (this.revealWanted && this.stats.loaded >= this.stats.wanted / 4)) {
      this.backdrop.beginReveal(performance.now());
    }
  }

  stopSway(): void {
    if (this.swayOrigin) this.options.camera.position.copy(this.swayOrigin);
    this.swayOrigin = undefined;
  }

  moveCamera(now: number): void {
    if (!this.swayOrigin) return;
    const t = now / 1000;
    this.options.camera.position.set(
      this.swayOrigin.x + Math.sin(t * 0.11) * 3.2,
      this.swayOrigin.y + Math.sin(t * 0.17) * 0.5,
      this.swayOrigin.z + Math.cos(t * 0.07) * 1.5,
    );
  }

  dispose(): void {
    this.events.abort();
    this.backdrop?.dispose();
  }
}
