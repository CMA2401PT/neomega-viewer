import * as THREE from "three";
import {
  AxisGizmo,
  SectionScene,
  createEffectRegistry,
  loadAtlas,
} from "@neomega/viewer/render";
import { FlightControls } from "./flight.ts";
import { SectionLoader } from "./stream.ts";

async function main(): Promise<void> {
  const atlas = await loadAtlas("/textures/atlas.png");
  const book = await loadAtlas("/textures/entity/enchanting_table_book.png");
  const scene = new THREE.Scene();
  scene.background = new THREE.Color(0xd9e5ed);
  scene.fog = new THREE.Fog(0xd9e5ed, 32, 48);
  const renderer = new THREE.WebGLRenderer({ antialias: true });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
  document.body.append(renderer.domElement);
  const camera = new THREE.PerspectiveCamera(65, 1, 0.1, 96);
  camera.position.set(8, 15, 24);
  const controls = new FlightControls(camera, renderer.domElement);
  const axes = new AxisGizmo(document.querySelector<HTMLCanvasElement>("#axes")!);
  const position = document.querySelector<HTMLOutputElement>("#position")!;

  // 核心负责静态几何合批、视锥/遮挡裁剪和可见书本的动画。
  const world = new SectionScene(scene, atlas, createEffectRegistry(book), renderer);
  const loader = new SectionLoader(world);
  function resize(): void {
    renderer.setSize(window.innerWidth, window.innerHeight);
    camera.aspect = window.innerWidth / window.innerHeight;
    camera.updateProjectionMatrix();
  }
  window.addEventListener("resize", resize);
  resize();
  let previousTime = performance.now(), lastHUD = 0;
  renderer.setAnimationLoop((time: number) => {
    // 切回标签页时不把暂停的时间算成一次大幅移动。
    controls.update(Math.min((time - previousTime) / 1000, 0.05));
    previousTime = time;
    loader.update(camera.position);
    world.update(camera, time / 1000);
    world.render(renderer, camera);
    if (time - lastHUD >= 100) {
      axes.update(camera.quaternion);
      position.textContent = loader.error || [
        `X ${camera.position.x.toFixed(1)}`,
        `Y ${camera.position.y.toFixed(1)}`,
        `Z ${camera.position.z.toFixed(1)}`,
      ].join("\n");
      lastHUD = time;
    }
  });
  window.addEventListener("pagehide", () => {
    renderer.setAnimationLoop(null);
    loader.dispose();
    world.dispose();
    axes.dispose();
    atlas.dispose();
    book.dispose();
    renderer.dispose();
  }, { once: true });
}

main().catch((error: unknown) => {
  console.error(error);
  document.querySelector<HTMLOutputElement>("#position")!.textContent = String(error);
});
