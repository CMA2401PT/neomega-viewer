/** 把 HUD 弹出的面板摆在触发按钮正上方，靠近屏幕边缘时往里收。开关和点外部关闭交给原生 popover。 */
export function anchorPopovers(hud: HTMLElement): void {
  const popovers = [...document.querySelectorAll<HTMLElement>(".popover")];
  function place(popover: HTMLElement): void {
    // 同一面板可能有两个触发按钮（宽屏的场景名、竖屏的 i），以当前看得见的那个为准。
    const trigger = [...document.querySelectorAll<HTMLElement>(`[popovertarget="${popover.id}"]`)]
      .find((button) => button.getClientRects().length > 0);
    if (!trigger) return;
    const button = trigger.getBoundingClientRect();
    const width = popover.offsetWidth;
    const left = Math.min(Math.max(button.left + button.width / 2 - width / 2, 8), innerWidth - width - 8);
    popover.style.left = `${left}px`;
    popover.style.bottom = `${innerHeight - hud.getBoundingClientRect().top + 8}px`;
    popover.dataset.placed = "";
  }
  for (const popover of popovers) {
    popover.addEventListener("toggle", (event) => {
      if ((event as ToggleEvent).newState === "open") place(popover);
      else delete popover.dataset.placed;
    });
  }
  window.addEventListener("resize", () => {
    for (const popover of popovers) if (popover.matches(":popover-open")) place(popover);
  });
}

/** 收起所有 HUD 弹出面板，隐藏界面或打开全屏地图时调用。 */
export function closePopovers(): void {
  for (const popover of document.querySelectorAll<HTMLElement>(".popover:popover-open")) popover.hidePopover();
}
