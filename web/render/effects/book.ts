import * as THREE from "three";
import type { EffectMessage, Vec3 } from "../types.ts";
import type { EffectObject } from "./index.ts";

/** BookModel 的 64×32 展开布局；模型像素换算为 1/16 格。 */
export function createEnchantingTableBook(
  message: EffectMessage,
  texture: THREE.Texture,
): EffectObject {
  const object = new THREE.Group();
  const book = new THREE.Group();
  book.scale.setScalar(1 / 16);
  book.rotation.z = (80 * Math.PI) / 180;
  object.add(book);
  const material = new THREE.MeshBasicMaterial({
    map: texture,
    side: THREE.DoubleSide,
    toneMapped: false,
    alphaTest: 0.1,
  });
  const geometries: THREE.BoxGeometry[] = [];
  const part = (
    origin: Vec3,
    size: Vec3,
    uv: [number, number],
    pivot: Vec3 = [0, 0, 0],
  ): THREE.Group => {
    const [x, y, z] = size;
    const [u, v] = uv;
    const geometry = new THREE.BoxGeometry(x, y, z);
    // 按立方体的六个面取 UV，封面内外与纸张侧边使用各自区域。
    const faces = [
      [u + z + x, v + z, z, y],
      [u, v + z, z, y],
      [u + z, v, x, z],
      [u + z + x, v, x, z],
      [u + 2 * z + x, v + z, x, y],
      [u + z, v + z, x, y],
    ];
    const attribute = geometry.getAttribute("uv");
    faces.forEach(([left, top, width, height], face) => {
      attribute.setXY(face * 4, left / 64, top / 32);
      attribute.setXY(face * 4 + 1, (left + width) / 64, top / 32);
      attribute.setXY(face * 4 + 2, left / 64, (top + height) / 32);
      attribute.setXY(face * 4 + 3, (left + width) / 64, (top + height) / 32);
    });
    const mesh = new THREE.Mesh(geometry, material);
    mesh.position.set(origin[0] + x / 2, origin[1] + y / 2, origin[2] + z / 2);
    const group = new THREE.Group();
    group.position.set(...pivot);
    group.add(mesh);
    book.add(group);
    geometries.push(geometry);
    return group;
  };
  const leftCover = part(
    [-6, -5, -0.005],
    [6, 10, 0.005],
    [0, 0],
    [0, 0, -1],
  );
  const rightCover = part(
    [0, -5, -0.005],
    [6, 10, 0.005],
    [16, 0],
    [0, 0, 1],
  );
  const seam = part([-1, -5, 0], [2, 10, 0.005], [12, 0]);
  seam.rotation.y = Math.PI / 2;
  const leftPages = part([0, -4, -0.99], [5, 8, 1], [0, 10]);
  const rightPages = part([0, -4, -0.01], [5, 8, 1], [12, 10]);
  const flipping = [
    part([0, -4, 0], [5, 8, 0.005], [24, 10]),
    part([0, -4, 0], [5, 8, 0.005], [24, 10]),
  ];
  const [x, y, z] = message.pos;
  // 取旋转、起伏的完整范围，书脊始终高于 0.75 格的台面。
  const bounds = new THREE.Box3(
    new THREE.Vector3(x - 0.1, y + 0.95, z - 0.1),
    new THREE.Vector3(x + 1.1, y + 1.65, z + 1.1),
  );
  const update = (time: number): void => {
    object.position.set(x + 0.5, y + 1.1 + Math.sin(time * 1.6) * 0.05, z + 0.5);
    object.rotation.y = time * 0.3;
    const openness = 1.25 + Math.sin(time * 0.4) * 0.1;
    leftCover.rotation.y = Math.PI + openness;
    rightCover.rotation.y = -openness;
    leftPages.rotation.y = openness;
    rightPages.rotation.y = -openness;
    for (const pages of [leftPages, rightPages, ...flipping])
      pages.position.x = Math.sin(openness);
    flipping.forEach((page, i) => {
      const phase = (time * 0.35 + i * 0.5) % 1;
      page.rotation.y = openness * (1 - 2 * phase);
    });
  };
  update(0);
  return {
    object,
    bounds,
    update,
    dispose() {
      geometries.forEach((geometry) => geometry.dispose());
      material.dispose();
    },
  };
}
