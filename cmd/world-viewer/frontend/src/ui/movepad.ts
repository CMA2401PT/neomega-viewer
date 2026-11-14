import type { FlightControls, Move } from "../scene/flight.ts";

/** 触屏上的移动按钮：按住期间持续移动，手指滑出按钮或被系统打断时停下。 */
export function bindMovePad(pad: HTMLElement, controls: FlightControls): void {
  for (const button of pad.querySelectorAll<HTMLButtonElement>("[data-move]")) {
    const move = button.dataset.move as Move;
    const release = (): void => {
      controls.hold(move, false);
      delete button.dataset.held;
    };
    button.addEventListener("pointerdown", (event) => {
      // 不让按钮抢走焦点，否则画布失焦会清掉正在按住的键盘方向。
      event.preventDefault();
      button.setPointerCapture(event.pointerId);
      button.dataset.held = "";
      controls.hold(move, true);
    });
    button.addEventListener("pointerup", release);
    button.addEventListener("pointercancel", release);
    button.addEventListener("lostpointercapture", release);
    button.addEventListener("contextmenu", (event) => event.preventDefault());
  }
}
