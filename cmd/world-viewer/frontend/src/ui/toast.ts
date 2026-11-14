const toast = document.getElementById("toast") as HTMLOutputElement;
let timer = 0;

/** 在 HUD 上方短暂显示一条提示，用于坐标越界、打开失败这类一次性的消息。 */
export function showToast(message: string): void {
  toast.value = message;
  toast.hidden = false;
  window.clearTimeout(timer);
  timer = window.setTimeout(() => { toast.hidden = true; }, 4000);
}
