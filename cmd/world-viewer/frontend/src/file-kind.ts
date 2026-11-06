export interface FileAppearance {
  icon: "folder" | "recognized" | "unknown";
  label: string;
}

/** 格式分类由服务端提供，前端只负责图标和说明，不另行猜测哪些格式能打开。 */
export function fileAppearance(kind: string, directory: boolean): FileAppearance {
  if (directory) return { icon: "folder", label: "文件夹" };
  const labels: Record<string, string> = {
    schem: "SCHEM · Java 结构",
    schematic: "SCHEMATIC · Java 结构",
    litematic: "LITEMATIC · Java 投影",
    nbt: "NBT · Java 结构",
    mcstructure: "MCSTRUCTURE · 基岩结构",
    mcworld: "MCWORLD · 基岩世界包",
    level: "存档入口",
    pack: "Java PackFS 存档",
  };
  return labels[kind]
    ? { icon: "recognized", label: labels[kind] }
    : { icon: "unknown", label: "未识别后缀" };
}

/** 用原生 SVG 绘制三种轮廓，图标含义同时由旁边的文字表达。 */
export function fileIcon(kind: FileAppearance["icon"]): SVGSVGElement {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("aria-hidden", "true");
  svg.classList.add("file-icon", `file-icon-${kind}`);
  const path = document.createElementNS(svg.namespaceURI, "path");
  const paths = {
    folder: "M3 6a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v10H3Z",
    recognized: "m12 3 9 5v9l-9 5-9-5V8Zm0 10 9-5M12 13 3 8m9 5v9M7.5 5.5l9 5v4",
    unknown: "M14 3H5v18h14V8Zm0 0v5h5M9 12a3 3 0 0 1 5 1c0 2-2 2-2 3m0 2h.01",
  };
  path.setAttribute("d", paths[kind]);
  svg.append(path);
  return svg;
}
