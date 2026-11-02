import * as THREE from "three";
import { OrbitControls } from "three/addons/controls/OrbitControls.js";
import { createMeshObject, decodeMesh, loadAtlas } from "@neomega/viewer/render";

async function main(): Promise<void> {
  const atlas = await loadAtlas("/textures/atlas.png");
  const response = await fetch("/api/subchunk?subchunk=default");
  if (!response.ok) throw new Error(await response.text());
  // 从go侧拿到mesh数据
  const binary = await response.arrayBuffer()
  // 核心只有这短短的两句
  const meshData = decodeMesh(binary)
  const mesh = createMeshObject(meshData, atlas);

  const scene = new THREE.Scene();
  scene.background = new THREE.Color(0xeeeeee);
  scene.add(mesh.object);

  const renderer = new THREE.WebGLRenderer({ antialias: true });
  document.body.append(renderer.domElement);
  const camera = new THREE.PerspectiveCamera(45, 1, 0.1, 100);
  camera.position.set(25, 20, 29);
  const controls = new OrbitControls(camera, renderer.domElement);
  controls.target.set(8, 4, 8);
  controls.update();

  // 场景没有动画，相机改变或窗口缩放时重绘即可。
  function render(): void {
    renderer.render(scene, camera);
  }
  function resize(): void {
    renderer.setSize(window.innerWidth, window.innerHeight);
    camera.aspect = window.innerWidth / window.innerHeight;
    camera.updateProjectionMatrix();
    render();
  }
  controls.addEventListener("change", render);
  window.addEventListener("resize", resize);
  resize();
}

main().catch((error: unknown) => {
  console.error(error);
  document.body.textContent = String(error);
});
