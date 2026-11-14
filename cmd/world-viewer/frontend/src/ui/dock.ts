// 下脚内凹圆角和上沿外凸圆角，单位 CSS 像素。斜边固定 45°，水平跨度等于底座高度；
// 内容左右留白（style.css 的 --dock-flare）按同样的高度算出，文字才不会压到斜边上。
const FOOT = 8;
const TOP = 6;
const SVG = "http://www.w3.org/2000/svg";
let docks = 0;

/** 两侧轮廓线：从页面下边缘内凹起脚，沿 45° 斜边上升，转过上沿圆角。closed 时沿下边缘闭合。 */
export function dockOutline(width: number, height: number, closed: boolean): string {
  const slant = height;
  const unit = Math.SQRT1_2;
  const n = (value: number): string => value.toFixed(2);
  const foot = [FOOT + FOOT * unit, height - FOOT * unit];
  const shoulder = [FOOT + slant - TOP * unit, TOP * unit];
  const left = `M0 ${n(height)}Q${n(FOOT)} ${n(height)} ${n(foot[0])} ${n(foot[1])}`
    + `L${n(shoulder[0])} ${n(shoulder[1])}Q${n(FOOT + slant)} 0 ${n(FOOT + slant + TOP)} 0`;
  const right = `L${n(width - FOOT - slant - TOP)} 0Q${n(width - FOOT - slant)} 0 ${n(width - shoulder[0])} ${n(shoulder[1])}`
    + `L${n(width - foot[0])} ${n(foot[1])}Q${n(width - FOOT)} ${n(height)} ${n(width)} ${n(height)}`;
  return left + right + (closed ? "Z" : "");
}

/** 给贴在页面下边缘的 .dock 画梯形底座：模糊层按轮廓裁剪，描边只画两侧和上沿，下沿融进页面边缘。 */
export function mountDock(dock: HTMLElement): void {
  const blur = document.createElement("div");
  blur.className = "dock-blur";
  const svg = document.createElementNS(SVG, "svg");
  svg.classList.add("dock-shape");
  svg.setAttribute("aria-hidden", "true");
  const create = <K extends keyof SVGElementTagNameMap>(tag: K, attributes: Record<string, string>): SVGElementTagNameMap[K] => {
    const node = document.createElementNS(SVG, tag);
    for (const [name, value] of Object.entries(attributes)) node.setAttribute(name, value);
    return node;
  };
  // 阴影只画在底座外面：底色半透明，阴影透进来会让底座发灰发脏。滤镜先于遮罩执行，遮罩边缘不会被模糊。
  const id = `dock-${++docks}`;
  const blurFilter = create("filter", { id: `${id}-shadow`, filterUnits: "userSpaceOnUse" });
  blurFilter.append(
    create("feGaussianBlur", { stdDeviation: "5" }),
    create("feOffset", { dy: "2" }),
  );
  const outside = create("rect", { fill: "#fff" });
  const inside = create("path", { fill: "#000" });
  const mask = create("mask", { id: `${id}-outside`, maskUnits: "userSpaceOnUse" });
  mask.append(outside, inside);
  const defs = create("defs", {});
  defs.append(blurFilter, mask);
  const shadow = create("path", {
    class: "dock-shadow",
    filter: `url(#${id}-shadow)`,
    mask: `url(#${id}-outside)`,
  });
  const fill = create("path", { class: "dock-fill" });
  const edge = create("path", { class: "dock-edge" });
  svg.append(defs, shadow, fill, edge);
  dock.prepend(blur, svg);
  new ResizeObserver(() => {
    const { offsetWidth: width, offsetHeight: height } = dock;
    if (!width || !height) return;
    const closed = dockOutline(width, height, true);
    svg.setAttribute("viewBox", `0 0 ${width} ${height}`);
    // 滤镜和遮罩的范围向外留出阴影的模糊距离。
    const region = { x: "-24", y: "-24", width: String(width + 48), height: String(height + 48) };
    for (const node of [blurFilter, mask, outside]) {
      for (const [name, value] of Object.entries(region)) node.setAttribute(name, value);
    }
    inside.setAttribute("d", closed);
    shadow.setAttribute("d", closed);
    fill.setAttribute("d", closed);
    // 描边宽 1.5px，向下错开半个线宽让上沿完整落在底座内；下端超出的部分被页面边缘裁掉。
    edge.setAttribute("d", dockOutline(width, height, false));
    edge.setAttribute("transform", "translate(0 0.75)");
    blur.style.clipPath = `path("${closed}")`;
  }).observe(dock);
}
