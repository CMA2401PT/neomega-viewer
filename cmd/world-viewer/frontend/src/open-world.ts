import type { WorldInfo } from "./picker.ts";

export interface OpenProgress {
  stage: string;
  completed: number;
  total: number;
  elapsedSeconds: number;
}

type OpenEvent =
  | { type: "progress"; progress: OpenProgress }
  | { type: "complete"; info: WorldInfo }
  | { type: "error"; error: string };

/** 读取同一次打开请求的进度流；仅在服务器发布新存档后返回，断流不视为成功。 */
export async function openWorld(
  path: string,
  progress: (value: OpenProgress) => void,
): Promise<WorldInfo> {
  const response = await fetch("/api/world", {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/x-ndjson" },
    body: JSON.stringify({ path }),
  });
  if (!response.ok) throw new Error(await response.text());
  if (!response.body) throw new Error("打开请求没有返回数据");
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let pending = "";
  try {
    for (;;) {
      const { value, done } = await reader.read();
      pending += decoder.decode(value, { stream: !done });
      let end: number;
      while ((end = pending.indexOf("\n")) >= 0) {
        const line = pending.slice(0, end);
        pending = pending.slice(end + 1);
        if (!line.trim()) continue;
        const event: OpenEvent = JSON.parse(line);
        if (event.type === "error") throw new Error(event.error);
        if (event.type === "complete") return event.info;
        progress(event.progress);
      }
      if (done) throw new Error("连接已中断，未收到存档打开结果");
    }
  } finally {
    await reader.cancel();
    reader.releaseLock();
  }
}

/** 百分比只描述当前阶段，整理阶段使用不定进度，避免假装知道剩余耗时。 */
export function progressLabel(value: OpenProgress): string {
  const labels: Record<string, string> = {
    opening: "正在打开存档",
    reading: "读取并解压结构",
    arranging: "整理方块",
    writing: "写入目标存档",
    extracting: "解压基岩世界包",
    finishing: "保存转换结果",
    complete: "转换完成，正在打开预览",
  };
  const label = labels[value.stage] ?? "正在转换";
  if (value.total <= 0 || value.stage === "complete") return label;
  const percent = Math.min(100, Math.floor(value.completed / value.total * 100));
  return `${label} · 本阶段 ${percent}%`;
}
