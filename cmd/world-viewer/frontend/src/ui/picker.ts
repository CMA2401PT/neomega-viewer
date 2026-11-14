import { fileAppearance, fileIcon } from "./file-kind.ts";
import type { WorldInfo } from "../api/world-info.ts";
import { openWorld, progressLabel, type OpenProgress } from "../api/open-world.ts";

interface Entry {
  name: string;
  path: string;
  directory: boolean;
  kind: string;
}
interface Listing {
  path: string;
  parent: string;
  selected: string;
  roots: Entry[];
  entries: Entry[];
}

/** 浏览服务所在电脑的文件系统，路径原样交回 Go，保留盘符与 UNC 路径语义。返回打开选择框的函数。 */
export function createWorldPicker(
  opened: (info: WorldInfo) => void,
  current: () => WorldInfo | undefined,
): () => void {
  const element = <T extends HTMLElement>(id: string): T =>
    document.getElementById(id) as T;
  const dialog = element<HTMLDialogElement>("world-picker");
  const address = element<HTMLInputElement>("picker-path");
  const roots = element<HTMLSelectElement>("picker-roots");
  const files = element<HTMLDivElement>("picker-files");
  const error = element<HTMLOutputElement>("picker-error");
  const selection = element<HTMLOutputElement>("picker-selection");
  const up = element<HTMLButtonElement>("picker-up");
  const openButton = element<HTMLButtonElement>("picker-open");
  const progressPanel = element<HTMLDivElement>("picker-progress");
  const progressBar = element<HTMLProgressElement>("picker-progress-bar");
  const progressText = element<HTMLOutputElement>("picker-progress-text");
  // 当前目录的条目与对应按钮，方向键在其中移动选中项。
  let rows: { entry: Entry; button: HTMLButtonElement }[] = [];
  let selected = "",
    parent = "",
    switching = false;
  let browsing: AbortController | undefined;

  function select(path: string): void {
    selected = path;
    selection.value = path;
    for (const { entry, button } of rows) {
      const active = entry.path === path;
      button.setAttribute("aria-selected", String(active));
      if (active) button.scrollIntoView({ block: "nearest" });
    }
  }
  function activate(entry: Entry): void {
    select(entry.path);
    if (entry.directory) void browse(entry.path);
    else void open();
  }
  async function browse(path: string): Promise<void> {
    browsing?.abort();
    const controller = new AbortController();
    browsing = controller;
    error.value = "";
    files.dataset.loading = "";
    try {
      const response = await fetch(
        `/api/files?${new URLSearchParams({ path })}`,
        { signal: controller.signal, cache: "no-store" },
      );
      if (!response.ok) throw new Error(await response.text());
      const listing: Listing = await response.json();
      if (controller.signal.aborted) return;
      address.value = listing.path;
      parent = listing.parent;
      up.disabled = parent === listing.path;
      roots.replaceChildren(
        new Option("位置", ""),
        ...listing.roots.map((entry) => new Option(entry.name, entry.path)),
      );
      rows = listing.entries.map((entry) => {
        const button = document.createElement("button");
        button.type = "button";
        button.role = "option";
        button.tabIndex = -1;
        const appearance = fileAppearance(entry.kind, entry.directory);
        const name = document.createElement("span");
        name.className = "file-name";
        name.textContent = entry.name;
        const kind = document.createElement("small");
        kind.textContent = appearance.label;
        button.title = entry.name;
        button.dataset.kind = appearance.icon;
        button.append(fileIcon(appearance.icon), name, kind);
        button.addEventListener("click", () => select(entry.path));
        button.addEventListener("dblclick", () => activate(entry));
        return { entry, button };
      });
      files.replaceChildren(...rows.map((row) => row.button));
      if (!rows.length) files.textContent = "空文件夹";
      select(listing.selected || listing.path);
    } catch (failure) {
      if (!controller.signal.aborted)
        error.value = failure instanceof Error ? failure.message : String(failure);
    } finally {
      if (browsing === controller) delete files.dataset.loading;
    }
  }
  async function open(): Promise<void> {
    if (switching) return;
    browsing?.abort();
    switching = true;
    dialog.dataset.busy = "";
    openButton.disabled = true;
    error.value = "";
    progressPanel.hidden = false;
    const started = performance.now();
    let received = started;
    let progress: OpenProgress = { stage: "opening", completed: 0, total: 0, elapsedSeconds: 0 };
    function renderProgress(): void {
      const converting = progress.stage !== "opening";
      const seconds = converting
        ? progress.elapsedSeconds + (progress.stage === "complete" ? 0 : (performance.now() - received) / 1000)
        : (performance.now() - started) / 1000;
      progressText.value = `${progressLabel(progress)} · ${converting ? "转换已用" : "已用"} ${seconds.toFixed(1)} 秒`;
      if (progress.total > 0) progressBar.value = progress.completed / progress.total;
      else progressBar.removeAttribute("value");
    }
    renderProgress();
    const timer = window.setInterval(renderProgress, 100);
    try {
      const info = await openWorld(selected || address.value, (value) => {
        progress = value;
        received = performance.now();
        renderProgress();
      });
      opened(info);
      dialog.close();
    } catch (failure) {
      error.value = `打开失败（已用 ${((performance.now() - started) / 1000).toFixed(1)} 秒）：${failure instanceof Error ? failure.message : String(failure)}`;
    } finally {
      window.clearInterval(timer);
      progressPanel.hidden = true;
      switching = false;
      delete dialog.dataset.busy;
      openButton.disabled = false;
    }
  }
  dialog.addEventListener("keydown", (event) => {
    if (switching) return;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const index = rows.findIndex((row) => row.entry.path === selected);
      const next = event.key === "ArrowDown"
        ? Math.min(index + 1, rows.length - 1)
        : Math.max(index - 1, 0);
      if (rows[next]) select(rows[next].entry.path);
      return;
    }
    if (event.key !== "Enter" || event.target === address) return;
    event.preventDefault();
    const row = rows.find((candidate) => candidate.entry.path === selected);
    if (row) activate(row.entry);
    else void open();
  });
  element<HTMLFormElement>("browse-path").addEventListener("submit", (event) => {
    // 地址栏回车：改过路径就转到该路径；没改过且已用方向键选中条目，就打开该条目。
    event.preventDefault();
    if (switching) return;
    const row = rows.find((candidate) => candidate.entry.path === selected);
    if (row && selected !== address.value) activate(row.entry);
    else void browse(address.value);
  });
  address.addEventListener("input", () => select(""));
  roots.addEventListener("change", () => {
    if (roots.value) void browse(roots.value);
  });
  up.addEventListener("click", () => void browse(parent));
  openButton.addEventListener("click", () => void open());
  element<HTMLButtonElement>("picker-cancel").addEventListener("click", () => dialog.close());
  dialog.addEventListener("cancel", (event) => {
    if (switching) event.preventDefault();
  });
  dialog.addEventListener("close", () => browsing?.abort());
  return () => {
    dialog.showModal();
    address.focus();
    void browse(current()?.path ?? "");
  };
}
