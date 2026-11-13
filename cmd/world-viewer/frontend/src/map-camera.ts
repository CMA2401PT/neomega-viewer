import { cameraBasis, type Camera, type Vec3 } from "../../../../../../neomega-viewer-core/viewer/brickmap/web/volume.ts";
import { cameraRay } from "../../../../../../neomega-viewer-core/viewer/brickmap/web/renderer.ts";

export function mapPoint(camera: Camera, width: number, height: number, x: number, y: number, planeY = camera.target[1]): Vec3 {
  // 拖动与缩放共用水平参考面，转过相机之后光标锚点仍保持在原处。
  const ray = cameraRay(camera, width, height, x, y);
  const t = (planeY - ray.origin[1]) / ray.direction[1];
  return ray.origin.map((n, i) => n + ray.direction[i] * t) as Vec3;
}

export function mapScreen(camera: Camera, width: number, height: number, point: Vec3): [number, number] {
  // 标记与占用图使用相同正交投影，方向不再假定北方始终位于屏幕上方。
  const { right, up } = cameraBasis(camera);
  const delta = point.map((n, i) => n - camera.target[i]);
  const scale = height / camera.span;
  return [
    width / 2 + delta.reduce((sum, n, i) => sum + n * right[i], 0) * scale,
    height / 2 - delta.reduce((sum, n, i) => sum + n * up[i], 0) * scale,
  ];
}

export function horizontalBounds(camera: Camera, width: number, height: number, minY: number, maxY: number): [number, number, number, number] {
  // 同时投影高度范围的上下端，避免斜视时屏幕角落的射线越出数据窗口。
  const points = [minY, maxY].flatMap(y => [0, width].flatMap(x =>
    [0, height].map(screenY => mapPoint(camera, width, height, x, screenY, y))));
  return [Math.min(...points.map(p => p[0])), Math.min(...points.map(p => p[2])),
    Math.max(...points.map(p => p[0])), Math.max(...points.map(p => p[2]))];
}
