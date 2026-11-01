import { decodeFrame } from "../../../../web/render/binary.ts";
import type { FrameData } from "../../../../web/render/types.ts";

export interface RenderIssue {
  position: [number, number, number];
  blockstate: string;
  reason: "missing_model" | "invalid_block_entities" | "invalid_block_entity" | "actor_error";
  detail?: string;
}

export function decodeColumn(buffer: ArrayBuffer): {
  sections: { y: number; bytes: number; unknown: number; frame: FrameData }[];
  unknown: number;
  issues: RenderIssue[];
} {
  if (buffer.byteLength < 20 || buffer.byteLength > 24 * 1024 * 1024)
    throw new Error("区块响应长度无效");
  const view = new DataView(buffer);
  if (view.getUint32(0, false) !== 0x4e565743 || view.getUint32(4, true) !== 3)
    throw new Error("区块响应格式无效");
  const count = view.getUint32(8, true),
    unknown = view.getUint32(12, true);
  if (count > 256) throw new Error("区块高度数量无效");
  const diagnosticsLength = view.getUint32(16, true);
  let offset = 20;
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
  if (offset + diagnosticsLength !== buffer.byteLength) throw new Error("区块响应包含多余数据");
  const issues: RenderIssue[] = diagnosticsLength
    ? JSON.parse(new TextDecoder().decode(buffer.slice(offset)))
    : [];
  return { sections, unknown, issues };
}
