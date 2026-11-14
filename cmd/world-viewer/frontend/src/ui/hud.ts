import * as THREE from "three";
import { FrameRate, geometryMemoryBytes } from "../scene/metrics.ts";

export class HUD {
  // 状态显示按自己的频率刷新，绘制循环只提供相机和场景。
  private readonly coordinates = document.getElementById("coordinates") as HTMLOutputElement;
  private readonly fps = document.getElementById("fps") as HTMLOutputElement;
  private readonly memory = document.getElementById("memory") as HTMLOutputElement;
  private readonly axes = document.getElementById("axes") as HTMLCanvasElement;
  private readonly axisContext = this.axes.getContext("2d")!;
  private readonly frameRate = new FrameRate();
  private lastUpdate = 0;
  private readonly reset = (): void => {
    this.frameRate.reset();
    this.fps.value = "FPS —";
  };

  constructor() {
    this.axes.width = this.axes.height = 80 * Math.min(devicePixelRatio, 2);
    document.addEventListener("visibilitychange", this.reset);
  }

  clearMemory(): void { this.memory.value = "网格内存 ≈ 0.0 MiB"; }

  update(now: number, camera: THREE.Camera, scene: THREE.Scene): void {
    const fps = this.frameRate.sample(now);
    if (fps !== undefined) {
      this.fps.value = `FPS ${fps.toFixed(0)}`;
      this.memory.value = `网格内存 ≈ ${(geometryMemoryBytes(scene) / 1024 ** 2).toFixed(1)} MiB`;
    }
    if (now - this.lastUpdate <= 80) return;
    this.drawAxes(camera);
    this.coordinates.value = `X ${camera.position.x.toFixed(1)}　Y ${camera.position.y.toFixed(1)}　Z ${camera.position.z.toFixed(1)}`;
    this.lastUpdate = now;
  }

  dispose(): void { document.removeEventListener("visibilitychange", this.reset); }

  private drawAxes(camera: THREE.Camera): void {
    const scale = this.axes.width / 80;
    this.axisContext.setTransform(scale, 0, 0, scale, 0, 0);
    this.axisContext.clearRect(0, 0, 80, 80);
    const inverse = camera.quaternion.clone().invert();
    const directions = [
      new THREE.Vector3(1, 0, 0),
      new THREE.Vector3(0, 1, 0),
      new THREE.Vector3(0, 0, 1),
    ]
      .map((v, i) => ({
        v: v.applyQuaternion(inverse),
        label: "XYZ"[i],
        color: ["#b63c3c", "#36733c", "#315b9a"][i],
      }))
      .sort((a, b) => a.v.z - b.v.z);
    this.axisContext.font = "12px ui-monospace,monospace";
    this.axisContext.textAlign = "center";
    this.axisContext.textBaseline = "middle";
    for (const { v, label, color } of directions) {
      this.axisContext.strokeStyle = this.axisContext.fillStyle = color;
      this.axisContext.lineWidth = 2;
      this.axisContext.beginPath();
      this.axisContext.moveTo(38, 40);
      this.axisContext.lineTo(38 + v.x * 24, 40 - v.y * 24);
      this.axisContext.stroke();
      this.axisContext.fillText(label, 38 + v.x * 33, 40 - v.y * 33);
    }
  }
}
