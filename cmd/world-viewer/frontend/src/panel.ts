/** 控制面板沿对角线滑出屏幕，只留下左上角的尖角作为把手。 */
export function createPanelToggle(): void {
  const panel = document.getElementById("panel")!;
  const body = document.getElementById("panel-body")!;
  const tip = document.getElementById("panel-tip") as HTMLButtonElement;
  tip.addEventListener("click", () => {
    const collapsed = panel.toggleAttribute("data-collapsed");
    // 收起后内容仍在 DOM 里，inert 避免 Tab 焦点落到屏幕外。
    body.inert = collapsed;
    tip.setAttribute("aria-expanded", String(!collapsed));
    tip.title = collapsed ? "展开控制面板" : "收起控制面板";
  });
}
