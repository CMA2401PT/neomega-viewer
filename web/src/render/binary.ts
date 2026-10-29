import type { Batch, MeshData, Surface } from "./types";

const HEADER_SIZE = 44;
const littleEndian = new Uint8Array(new Uint16Array([1]).buffer)[0] === 1;

/** Decode NVMS v1. Numeric attributes view the response buffer on little-endian
 * hosts; the material table is the only JSON. No block/model parsing occurs. */
export function decodeMesh(buffer: ArrayBuffer): MeshData {
  const view = new DataView(buffer);
  const text = new TextDecoder();
  if (
    buffer.byteLength < HEADER_SIZE ||
    text.decode(new Uint8Array(buffer, 0, 4)) !== "NVMS"
  ) {
    throw new Error("无效的 NVMS 网格文件");
  }
  if (view.getUint32(4, true) !== 1) throw new Error("不支持的 NVMS 版本");
  const metadataLength = view.getUint32(8, true);
  const batchCount = view.getUint32(12, true);
  const quads = view.getUint32(16, true);
  let offset = HEADER_SIZE + metadataLength;
  if (offset > buffer.byteLength) throw new Error("材质表不完整");
  const surfaces: Surface[] = JSON.parse(
    text.decode(new Uint8Array(buffer, HEADER_SIZE, metadataLength)),
  );
  if (!Array.isArray(surfaces) || surfaces.length !== batchCount)
    throw new Error("材质数量与批次数量不符");
  offset = Math.ceil(offset / 4) * 4;
  const takeFloats = (count: number): Float32Array => {
    if (offset + count * 4 > buffer.byteLength)
      throw new Error("顶点缓冲不完整");
    const values = littleEndian
      ? new Float32Array(buffer, offset, count)
      : Float32Array.from({ length: count }, (_, i) =>
          view.getFloat32(offset + i * 4, true),
        );
    offset += count * 4;
    return values;
  };
  const batches: Batch[] = [];
  let countedQuads = 0;
  for (const surface of surfaces) {
    if (offset + 8 > buffer.byteLength) throw new Error("材质批次头不完整");
    const vertices = view.getUint32(offset, true);
    const indexCount = view.getUint32(offset + 4, true);
    offset += 8;
    if (vertices % 4 !== 0 || indexCount !== (vertices / 4) * 6)
      throw new Error("批次不是四边形网格");
    const positions = takeFloats(vertices * 3);
    const normals = takeFloats(vertices * 3);
    const shadeNormals = takeFloats(vertices * 3);
    const uvs = takeFloats(vertices * 2);
    const colors = takeFloats(vertices * 3);
    if (offset + indexCount * 4 > buffer.byteLength)
      throw new Error("索引缓冲不完整");
    const indices = littleEndian
      ? new Uint32Array(buffer, offset, indexCount)
      : Uint32Array.from({ length: indexCount }, (_, i) =>
          view.getUint32(offset + i * 4, true),
        );
    offset += indexCount * 4;
    if (indices.some((index) => index >= vertices))
      throw new Error("索引超出顶点范围");
    batches.push({
      surface,
      positions,
      normals,
      shadeNormals,
      uvs,
      colors,
      indices,
    });
    countedQuads += vertices / 4;
  }
  if (offset !== buffer.byteLength || countedQuads !== quads)
    throw new Error("网格长度或面数与文件头不符");
  return {
    batches,
    bounds: {
      min: [
        view.getFloat32(20, true),
        view.getFloat32(24, true),
        view.getFloat32(28, true),
      ],
      max: [
        view.getFloat32(32, true),
        view.getFloat32(36, true),
        view.getFloat32(40, true),
      ],
    },
    quads,
  };
}
