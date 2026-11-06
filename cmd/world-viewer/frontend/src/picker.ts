import { fileAppearance, fileIcon } from "./file-kind.ts";
import { openWorld, progressLabel, type OpenProgress } from "./open-world.ts";

export interface WorldInfo {
  revision: number;
  canOpen?: boolean;
  loaded: boolean;
  name: string;
  path?: string;
  kind?: string;
  dimension?: string;
  conversionSeconds?: number;
  reusedStructure?: boolean;
  position: [number, number, number];
  resourceVersion: string;
}
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

/** 浏览服务所在电脑的文件系统，路径原样交回 Go，保留盘符与 UNC 路径语义。 */
export function createWorldPicker(
  opened: (info: WorldInfo) => void,
  current: () => WorldInfo | undefined,
): void {
  const element = <T extends HTMLElement>(id: string): T =>
    document.getElementById(id) as T;
  const dialog = element<HTMLDialogElement>("world-picker");
  const controls = element<HTMLFieldSetElement>("picker-controls");
  const address = element<HTMLInputElement>("picker-path");
  const roots = element<HTMLSelectElement>("picker-roots");
  const files = element<HTMLDivElement>("picker-files");
  const error = element<HTMLOutputElement>("picker-error");
  const selection = element<HTMLOutputElement>("picker-selection");
  const up = element<HTMLButtonElement>("picker-up");
  const progressPanel = element<HTMLDivElement>("picker-progress");
  const progressBar = element<HTMLProgressElement>("picker-progress-bar");
  const progressText = element<HTMLOutputElement>("picker-progress-text");
  let selected = "",
    parent = "",
    switching = false;
  let browsing: AbortController | undefined;

  function select(path: string): void {
    selected = path;
    selection.value = path;
    for (const button of files.querySelectorAll<HTMLButtonElement>("button"))
      button.setAttribute("aria-pressed", String(button.dataset.path === path));
  }
  async function browse(path: string): Promise<void> {
    browsing?.abort();
    const controller = new AbortController();
    browsing = controller;
    error.value = "读取目录…";
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
        new Option("位置…", ""),
        ...listing.roots.map((entry) => new Option(entry.name, entry.path)),
      );
      files.replaceChildren();
      for (const entry of listing.entries) {
        const button = document.createElement("button");
        button.type = "button";
        button.dataset.path = entry.path;
        const name = document.createElement("span");
        name.textContent = entry.name;
        const kind = document.createElement("small");
        const appearance = fileAppearance(entry.kind, entry.directory);
        name.className = "file-name";
        kind.textContent = appearance.label;
        button.title = `${entry.name} · ${appearance.label}`;
        button.append(fileIcon(appearance.icon), name, kind);
        button.addEventListener("click", () => select(entry.path));
        button.addEventListener("dblclick", () => {
          if (entry.directory) void browse(entry.path);
          else void open();
        });
        button.addEventListener("keydown", (event) => {
          if (event.key === "Enter") {
            event.preventDefault();
            select(entry.path);
            if (entry.directory) void browse(entry.path);
            else void open();
          }
        });
        files.append(button);
      }
      if (!listing.entries.length) files.textContent = "空文件夹";
      select(listing.selected || listing.path);
      error.value = "";
    } catch (failure) {
      if (!controller.signal.aborted)
        error.value =
          failure instanceof Error ? failure.message : String(failure);
    }
  }
  async function open(): Promise<void> {
    if (switching) return;
    browsing?.abort();
    switching = true;
    controls.disabled = true;
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
      controls.disabled = false;
    }
  }
  element<HTMLButtonElement>("open-world").addEventListener("click", () => {
    dialog.showModal();
    void browse(current()?.path ?? "");
  });
  element<HTMLFormElement>("browse-path").addEventListener(
    "submit",
    (event) => {
      event.preventDefault();
      void browse(address.value);
    },
  );
  address.addEventListener("input", () => select(""));
  roots.addEventListener("change", () => {
    if (roots.value) void browse(roots.value);
  });
  up.addEventListener("click", () => {
    void browse(parent);
  });
  element<HTMLButtonElement>("picker-open").addEventListener("click", () => {
    void open();
  });
  element<HTMLButtonElement>("picker-cancel").addEventListener("click", () =>
    dialog.close(),
  );
  dialog.addEventListener("cancel", (event) => {
    if (switching) event.preventDefault();
  });
  dialog.addEventListener("close", () => browsing?.abort());
}
