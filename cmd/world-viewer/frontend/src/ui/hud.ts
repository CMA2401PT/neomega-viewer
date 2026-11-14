import * as THREE from "three";
import { FrameRate, geometryMemoryBytes } from "../scene/metrics.ts";

const AXES = [
  { direction: new THREE.Vector3(1, 0, 0), color: "#d1453b", label: "X" },
  { direction: new THREE.Vector3(0, 1, 0), color: "#3f9a4b", label: "Y" },
  { direction: new THREE.Vector3(0, 0, 1), color: "#3a6fc4", label: "Z" },
];

export class HUD {
  // 读数按自己的频率刷新，绘制循环只提供相机和场景。
  private readonly fields = ["x", "y", "z"].map((id) => document.getElementById(id) as HTMLInputElement);
  private readonly teleport = document.getElementById("teleport") as HTMLFormElement;
  private readonly go = document.getElementById("teleport-go") as HTMLButtonElement;
  private readonly fps = document.getElementById("fps") as HTMLOutputElement;
  private readonly memory = document.getElementById("memory") as HTMLOutputElement;
  private readonly axes = document.getElementById("axes") as HTMLCanvasElement;
  private readonly axisContext = this.axes.getContext("2d")!;
  // 竖屏的 HUD 放不下小数，坐标只显示整数；断点与 style.css 的紧凑布局一致。
  private readonly compact = matchMedia("(max-width: 640px)");
  private readonly frameRate = new FrameRate();
  private readonly inverse = new THREE.Quaternion();
  private readonly axis = new THREE.Vector3();
  private readonly events = new AbortController();
  private editing = false;
  private lastUpdate = 0;

  constructor(jump: (position: [number, number, number]) => void) {
    const signal = this.events.signal;
    document.addEventListener("visibilitychange", () => {
      this.frameRate.reset();
      this.fps.value = "FPS —";
    }, { signal });
    // 点击坐标进入编辑：读数停止跟随相机，回车或箭头跳转，Esc 或点到别处放弃。
    this.teleport.addEventListener("focusin", (event) => {
      if (!this.editing) {
        this.editing = true;
        this.go.disabled = false;
        this.teleport.dataset.editing = "";
      }
      (event.target as HTMLInputElement).select?.();
    }, { signal });
    this.teleport.addEventListener("focusout", (event) => {
      if (!this.teleport.contains(event.relatedTarget as Node | null)) this.stopEditing();
    }, { signal });
    // Safari 点按钮不给按钮焦点，输入框会先失焦退出编辑；按下时留住焦点，点击才能提交。
    this.go.addEventListener("mousedown", (event) => event.preventDefault(), { signal });
    this.teleport.addEventListener("keydown", (event) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      (document.activeElement as HTMLElement | null)?.blur();
    }, { signal });
    this.teleport.addEventListener("submit", (event) => {
      event.preventDefault();
      const position = this.fields.map((field) => Number(field.value.trim())) as [number, number, number];
      this.stopEditing();
      jump(position);
    }, { signal });
  }

  clearMemory(): void { this.memory.value = "网格 0.0 MiB"; }

  update(now: number, camera: THREE.Camera, scene: THREE.Scene): void {
    const fps = this.frameRate.sample(now);
    if (fps !== undefined) {
      this.fps.value = `FPS ${fps.toFixed(0)}`;
      this.memory.value = `网格 ${(geometryMemoryBytes(scene) / 1024 ** 2).toFixed(1)} MiB`;
    }
    if (now - this.lastUpdate <= 80) return;
    this.lastUpdate = now;
    this.drawAxes(camera);
    if (!this.editing) this.showPosition(camera.position);
  }

  dispose(): void { this.events.abort(); }

  private stopEditing(): void {
    this.editing = false;
    this.go.disabled = true;
    delete this.teleport.dataset.editing;
    // 立即恢复为相机位置，不等下一次刷新，放弃编辑时不会闪出改到一半的数字。
    this.lastUpdate = 0;
  }

  private showPosition(position: THREE.Vector3): void {
    const compact = this.compact.matches;
    position.toArray().forEach((value, i) => {
      const text = compact ? String(Math.floor(value)) : value.toFixed(1);
      const field = this.fields[i];
      if (field.value === text) return;
      field.value = text;
      // 输入框宽度跟着数字长度走，HUD 不为最长的坐标预留空白。
      field.style.width = `calc(${Math.max(text.length, 2)}ch + 6px)`;
    });
  }

  private drawAxes(camera: THREE.Camera): void {
    // 正方向画到端点的实心圆并写上轴名，反方向只画淡色小圆；按深度从远到近画，近的盖住远的。
    const size = this.axes.clientWidth;
    if (!size) return;
    const ratio = Math.min(devicePixelRatio, 2);
    const pixels = Math.round(size * ratio);
    if (this.axes.width !== pixels) this.axes.width = this.axes.height = pixels;
    const context = this.axisContext;
    const center = size / 2;
    const reach = center - 10;
    context.setTransform(ratio, 0, 0, ratio, 0, 0);
    context.clearRect(0, 0, size, size);
    context.lineWidth = 2;
    context.lineCap = "round";
    context.textAlign = "center";
    context.textBaseline = "middle";
    context.font = "600 10px system-ui, -apple-system, sans-serif";
    this.inverse.copy(camera.quaternion).invert();
    const ends = AXES.flatMap(({ direction, color, label }) => {
      const v = this.axis.copy(direction).applyQuaternion(this.inverse);
      return [
        { x: v.x, y: v.y, z: v.z, color, label },
        { x: -v.x, y: -v.y, z: -v.z, color, label: "" },
      ];
    }).sort((a, b) => a.z - b.z);
    for (const { x, y, color, label } of ends) {
      const px = center + x * reach, py = center - y * reach;
      context.strokeStyle = context.fillStyle = color;
      if (label) {
        context.beginPath();
        context.moveTo(center, center);
        context.lineTo(px, py);
        context.stroke();
        context.beginPath();
        context.arc(px, py, 8, 0, Math.PI * 2);
        context.fill();
        context.fillStyle = "#fff";
        context.fillText(label, px, py + 0.5);
      } else {
        context.globalAlpha = 0.35;
        context.beginPath();
        context.arc(px, py, 5, 0, Math.PI * 2);
        context.fill();
        context.globalAlpha = 1;
      }
    }
  }
}
