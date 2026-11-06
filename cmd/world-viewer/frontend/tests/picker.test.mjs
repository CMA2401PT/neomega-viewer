import { test } from "node:test";
import assert from "node:assert/strict";
import { fileAppearance } from "../src/file-kind.ts";
import { openWorld, progressLabel } from "../src/open-world.ts";

test("文件夹、已识别格式和未知后缀使用不同图标", () => {
  assert.equal(fileAppearance("unknown", true).icon, "folder");
  for (const kind of ["schem", "schematic", "litematic", "nbt", "mcstructure", "mcworld", "level", "pack"])
    assert.equal(fileAppearance(kind, false).icon, "recognized");
  assert.deepEqual(fileAppearance("unknown", false), { icon: "unknown", label: "未识别后缀" });
});

test("打开响应跨字节分片时仍先交付进度，完成后才发布存档", async (t) => {
  const encoder = new TextEncoder();
  const events = [
    { type: "progress", progress: { stage: "reading", completed: 5, total: 10, elapsedSeconds: 0.2 } },
    { type: "complete", info: { name: "中文地图", conversionSeconds: 1.4 } },
  ];
  const bytes = encoder.encode(events.map((event) => JSON.stringify(event)).join("\n") + "\n");
  t.mock.method(globalThis, "fetch", async (url, options) => {
    assert.equal(url, "/api/world");
    assert.equal(options.headers.Accept, "application/x-ndjson");
    assert.deepEqual(JSON.parse(options.body), { path: "C:\\地图.schem" });
    return new Response(new ReadableStream({
      start(controller) {
        for (let offset = 0; offset < bytes.length; offset += 2) controller.enqueue(bytes.slice(offset, offset + 2));
        controller.close();
      },
    }));
  });
  const progress = [];
  const info = await openWorld("C:\\地图.schem", (value) => progress.push(value));
  assert.equal(progress.length, 1);
  assert.equal(progressLabel(progress[0]), "读取并解压结构 · 本阶段 50%");
  assert.equal(info.name, "中文地图");
  assert.equal(info.conversionSeconds, 1.4);
});

test("转换失败或未完成就断流时不返回新存档", async (t) => {
  const fetch = t.mock.method(globalThis, "fetch");
  fetch.mock.mockImplementation(async () => new Response('{"type":"error","error":"结构已损坏"}\n'));
  await assert.rejects(openWorld("bad.schem", () => {}), /结构已损坏/);
  fetch.mock.mockImplementation(async () => new Response('{"type":"progress","progress":{"stage":"arranging","total":0}}\n'));
  await assert.rejects(openWorld("broken.schem", () => {}), /连接已中断/);
  assert.equal(progressLabel({ stage: "arranging", total: 0 }), "整理方块");
});
