import type { RenderIssue } from "./packet.ts";

/** Keep the first observation throughout one opened world, even after unloading chunks. */
export function createDiagnostics(
  count: HTMLOutputElement,
  list: HTMLUListElement,
  empty: HTMLElement,
): { add(issues: RenderIssue[], column: [number, number]): void; clear(): void } {
  const seen = new Set<string>();
  return {
    add(issues, column) {
      // 同一列的来源条目会随每个高度窗口重复发送，同样的条目在一次响应内仍保留各自次数。
      const occurrences = new Map<string, number>();
      const fragment = document.createDocumentFragment();
      for (const issue of issues) {
        let key: string;
        if (issue.scope === "entity") {
          key = JSON.stringify(["entity", issue.position, issue.entityId, issue.reason, issue.detail]);
        } else if (issue.origin === "render") {
          key = JSON.stringify(["render", issue.blockstate, issue.reason]);
        } else {
          const entry = JSON.stringify([
            column, issue.scope, issue.position, issue.blockstate ?? "", issue.reason,
            issue.count, issue.blockEntityId ?? "", issue.detail ?? "",
          ]);
          const occurrence = occurrences.get(entry) ?? 0;
          occurrences.set(entry, occurrence + 1);
          key = JSON.stringify(["source", entry, occurrence]);
        }
        if (seen.has(key)) continue;
        seen.add(key);
        const item = document.createElement("li");
        const state = document.createElement("code");
        state.textContent = issue.blockstate || issue.blockEntityId || issue.entityId || "来源数据异常";
        const location = document.createElement("div");
        const [x, y, z] = issue.position;
        location.textContent = issue.scope !== "column"
          ? `首次发现：X ${x} · Y ${y} · Z ${z}`
          : `区块：${column[0]}, ${column[1]}`;
        const reason = document.createElement("div");
        const reasons: Record<string, string> = {
          missing_model: "当前资源缺少模型（紫色占位）",
          invalid_entities: "无法读取普通实体列表",
          invalid_entity: "已跳过异常的普通实体数据",
          entity_error: "实体外观生成失败，保留可用部分",
          invalid_block_entities: "无法读取方块实体列表，保留静态模型",
          invalid_block_entity: "已跳过异常的方块实体数据",
          unknown_block: "来源无法转换方块状态",
          unsupported_block_entity: "来源不支持此方块实体",
          actor_error: "特殊外观生成失败，保留静态模型",
        };
        reason.textContent = `${reasons[issue.reason] ?? issue.reason}${issue.count > 1 ? ` · ${issue.count} 个` : ""}`;
        item.append(state, location, reason);
        if (issue.detail) {
          const detail = document.createElement("div");
          detail.textContent = issue.detail;
          item.append(detail);
        }
        fragment.append(item);
      }
      list.append(fragment);
      count.value = String(seen.size);
      empty.hidden = seen.size > 0;
    },
    clear() {
      seen.clear();
      list.replaceChildren();
      count.value = "0";
      empty.hidden = false;
    },
  };
}
