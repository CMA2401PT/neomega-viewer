import { decodeFrame } from "../../../../web/render/binary.ts";
import type { FrameData } from "../../../../web/render/types.ts";

export function decodeColumn(buffer: ArrayBuffer): {
  sections: { y: number; bytes: number; unknown: number; frame: FrameData }[];
  unknown: number;
} {
  if (buffer.byteLength < 16 || buffer.byteLength > 24 * 1024 * 1024)
    throw new Error("区块响应长度无效");
  const view = new DataView(buffer);
  if (view.getUint32(0, false) !== 0x4e565743 || view.getUint32(4, true) !== 2)
    throw new Error("区块响应格式无效");
  const count = view.getUint32(8, true),
    unknown = view.getUint32(12, true);
  if (count > 256) throw new Error("区块高度数量无效");
  let offset = 16;
  const sections: {
    y: number;
    bytes: number;
    unknown: number;
    frame: FrameData;
  }[] = [];
  for (let i = 0; i < count; i++) {
    if (offset + 12 > buffer.byteLength) throw new Error("区块响应不完整");
    const y = view.getInt32(offset, true),
      length = view.getUint32(offset + 4, true);
    const missing = view.getUint32(offset + 8, true);
    offset += 12;
    if (y < -128 || y > 127 || length > buffer.byteLength - offset)
      throw new Error("section 长度或高度无效");
    sections.push({
      y,
      bytes: length,
      unknown: missing,
      frame: decodeFrame(buffer.slice(offset, offset + length)),
    });
    offset += length;
  }
  if (offset !== buffer.byteLength) throw new Error("区块响应包含多余数据");
  return { sections, unknown };
}
