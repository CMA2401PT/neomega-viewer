import * as THREE from "three";
import type { BackdropInfo } from "./backdrop.ts";

export interface Stage {
  show(info: BackdropInfo | undefined): void;
  update(camera: THREE.PerspectiveCamera, now: number): void;
}

/** 字标和标注贴在三维锚点上，随镜头移动，让界面和场景看起来是同一幅画。 */
export function createStage(): Stage {
  const stage = document.getElementById("stage")!;
  const title = document.getElementById("stage-title")!;
  const ringNote = document.getElementById("note-ring")!;
  const seaNote = document.getElementById("note-sea")!;
  const ringLeader = document.getElementById("stage-leader-ring")!;
  const seaLeader = document.getElementById("stage-leader-sea")!;
  const point = new THREE.Vector3();
  let info: BackdropInfo | undefined;

  /** 锚点转到镜头背后时返回 undefined，对应的标注随之隐藏。 */
  function project(
    camera: THREE.PerspectiveCamera,
    x: number,
    y: number,
    z: number,
  ): [number, number] | undefined {
    point.set(x, y, z).project(camera);
    if (point.z > 1) return undefined;
    return [(point.x + 1) * 0.5 * innerWidth, (1 - point.y) * 0.5 * innerHeight];
  }
  function place(element: HTMLElement, x: number, y: number): void {
    element.style.transform = `translate(${x.toFixed(1)}px, ${y.toFixed(1)}px)`;
  }
  return {
    show(next) {
      info = next;
      stage.hidden = !next;
      if (!next) {
        delete stage.dataset.visible;
        return;
      }
      ringNote.lastElementChild!.textContent = `${next.lanterns.toLocaleString("en-US")} SEA LANTERNS · ONE SIDE`;
      seaNote.lastElementChild!.textContent = `${next.glass.toLocaleString("en-US")} PANES · Y ${next.seaLevel}`;
    },
    update(camera, now) {
      if (!info) return;
      if (now > 1800) stage.dataset.visible = "";
      // 字标和入口站在环内、略高于海面；字号跟着环的屏幕尺寸走，--unit 是一格方块的像素。
      const [cx, , cz] = info.center;
      const base = project(camera, cx, info.seaLevel + info.radius * 0.16, cz);
      const edge = project(camera, cx + 1, info.seaLevel, cz);
      title.hidden = !base || !edge;
      if (base && edge) {
        stage.style.setProperty("--unit", `${Math.abs(edge[0] - base[0]).toFixed(3)}px`);
        place(title, base[0], base[1] - title.offsetHeight);
      }

      const twist = project(camera, ...info.twist);
      ringNote.hidden = !twist;
      ringLeader.toggleAttribute("hidden", !twist);
      if (twist) {
        const [twistX, twistY] = twist;
        const elbowX = twistX + innerHeight * 0.1;
        const elbowY = twistY - innerHeight * 0.1;
        ringLeader.setAttribute("d", `M${twistX} ${twistY}L${elbowX} ${elbowY}h70`);
        place(ringNote, elbowX + 8, elbowY - 34);
      }

      const sea = project(camera, -128, info.seaLevel, 90);
      seaNote.hidden = !sea;
      seaLeader.toggleAttribute("hidden", !sea);
      if (sea) {
        const [seaX, seaY] = sea;
        const noteY = seaY + innerHeight * 0.07;
        seaLeader.setAttribute("d", `M${seaX} ${seaY}L${seaX - 40} ${noteY}h-70`);
        place(seaNote, seaX - 118 - 150, noteY - 34);
      }
    },
  };
}
