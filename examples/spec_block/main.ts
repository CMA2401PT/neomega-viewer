import * as THREE from "three";
import { OrbitControls } from "three/addons/controls/OrbitControls.js";
import {
  createEffectRegistry,
  createFrameObject,
  decodeFrame,
  loadAtlas,
} from "@neomega/viewer/render";

async function main(): Promise<void> {
  const atlas = await loadAtlas("/textures/atlas.png");
  const book = await loadAtlas("/textures/entity/enchanting_table_book.png");
  const response = await fetch("/api/subchunk?subchunk=default");
  if (!response.ok) throw new Error(await response.text());
  // 完整快照包含静态网格、陶罐 actor 网格和书本 effect 消息。
  const binary = await response.arrayBuffer();
  const frameData = decodeFrame(binary);
  const frame = createFrameObject(frameData, atlas, createEffectRegistry(book));

  const scene = new THREE.Scene();
  scene.background = new THREE.Color(0xeeeeee);
  scene.add(frame.object);

  const renderer = new THREE.WebGLRenderer({ antialias: true });
  document.body.append(renderer.domElement);
  const camera = new THREE.PerspectiveCamera(45, 1, 0.1, 100);
  camera.position.set(25, 20, 29);
  const controls = new OrbitControls(camera, renderer.domElement);
  controls.target.set(8, 4, 8);
  controls.update();

  function resize(): void {
    renderer.setSize(window.innerWidth, window.innerHeight);
    camera.aspect = window.innerWidth / window.innerHeight;
    camera.updateProjectionMatrix();
  }
  window.addEventListener("resize", resize);
  resize();
  // 书本持续起伏、旋转和翻页，每帧传入同一个秒数再绘制。
  renderer.setAnimationLoop((time: number) => {
    frame.updateTime(time / 1000);
    renderer.render(scene, camera);
  });
}

main().catch((error: unknown) => {
  console.error(error);
  document.body.textContent = String(error);
});
