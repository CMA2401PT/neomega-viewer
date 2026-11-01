import type { RenderIssue } from "./packet.ts";

/** Keep the first observation throughout one opened world, even after unloading chunks. */
export function createDiagnostics(
  count: HTMLOutputElement,
  list: HTMLUListElement,
  empty: HTMLElement,
): { add(issues: RenderIssue[]): void; clear(): void } {
  const seen = new Set<string>();
  return {
    add(issues) {
      const fragment = document.createDocumentFragment();
      for (const issue of issues) {
        const key = issue.blockstate || `${issue.reason}:${issue.position.join(",")}`;
        if (seen.has(key)) continue;
        seen.add(key);
        const item = document.createElement("li");
        const state = document.createElement("code");
        state.textContent = issue.blockstate || "方块实体数据异常";
        const location = document.createElement("div");
        const [x, y, z] = issue.position;
        location.textContent = issue.blockstate
          ? `首次发现：X ${x} · Y ${y} · Z ${z}`
          : `区块：${x >> 4}, ${z >> 4}`;
        const reason = document.createElement("div");
        const reasons: Record<RenderIssue["reason"], string> = {
          missing_model: "当前资源缺少模型（紫色占位）",
          invalid_block_entities: "无法读取方块实体列表，保留静态模型",
          invalid_block_entity: "已跳过异常的方块实体数据",
          actor_error: "特殊外观生成失败，保留静态模型",
        };
        reason.textContent = reasons[issue.reason];
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
