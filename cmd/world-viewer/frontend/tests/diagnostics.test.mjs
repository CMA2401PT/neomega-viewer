import { test } from "node:test";
import assert from "node:assert/strict";
import { createDiagnostics } from "../src/diagnostics.ts";
import { decodeColumn } from "../src/packet.ts";

// 只实现面板使用的 DOM 操作，让断言落在显示条目与计数上。
class Element {
  children = [];
  textContent = "";
  append(...children) {
    for (const child of children)
      this.children.push(...(child.fragment ? child.children : [child]));
  }
  replaceChildren() { this.children = []; }
}

test("source diagnostics retain separate entries without multiplying across height windows", () => {
  const previous = globalThis.document;
  globalThis.document = {
    createDocumentFragment: () => Object.assign(new Element(), { fragment: true }),
    createElement: () => new Element(),
  };
  try {
    const count = { value: "0" }, list = new Element(), empty = { hidden: false };
    const diagnostics = createDiagnostics(count, list, empty);
    const issues = [
      { scope: "column", origin: "source", position: [-32, 0, 48], blockstate: "example:a []", reason: "unknown_block", count: 7 },
      { scope: "column", origin: "source", position: [-32, 0, 48], blockstate: "example:b []", reason: "unknown_block", count: 2 },
      { scope: "column", origin: "source", position: [-32, 0, 48], blockEntityId: "example:entity", reason: "unsupported_block_entity", count: 3 },
      { scope: "block", origin: "source", position: [-31, 200, 49], reason: "invalid_block_entity", count: 1, detail: "bad" },
    ];
    issues.push({ ...issues[3] });
    diagnostics.add(issues, [-2, 3]);
    diagnostics.add(issues.map((issue) => ({ ...issue })), [-2, 3]);
    assert.equal(count.value, "5");
    assert.equal(list.children.length, 5);
    assert.equal(list.children[0].children[1].textContent, "区块：-2, 3", "a state without a position is still column scoped");
    assert.match(list.children[0].children[2].textContent, /7 个/);
    assert.equal(list.children[2].children[0].textContent, "example:entity");
    assert.match(list.children[3].children[1].textContent, /Y 200/);
    diagnostics.add(issues.slice(0, 1), [-1, 3]);
    assert.equal(count.value, "6", "other columns must retain their own count");
    const render = { scope: "block", origin: "render", position: [1, 2, 3], blockstate: "example:a []", reason: "missing_model", count: 1 };
    diagnostics.add([render], [0, 0]);
    diagnostics.add([{ ...render, position: [16, 20, 30] }], [1, 1]);
    diagnostics.add([{ ...render, reason: "actor_error" }], [0, 0]);
    assert.equal(count.value, "8", "render states deduplicate by state and reason");
    const entity = { scope: "entity", origin: "render", position: [1, -60, 3], entityId: "minecraft:item_frame", reason: "entity_error", count: 1, detail: "missing item" };
    diagnostics.add([entity, { ...entity, position: [2, -60, 3] }], [0, 0]);
    diagnostics.add([entity], [0, 0]);
    assert.equal(count.value, "10", "entity diagnostics retain each position without repeating it");
    assert.equal(list.children[8].children[0].textContent, "minecraft:item_frame");
    assert.match(list.children[8].children[1].textContent, /Y -60/);
    assert.match(list.children[8].children[2].textContent, /实体外观/);
    diagnostics.clear();
    assert.equal(count.value, "0");
    assert.equal(empty.hidden, false);
    diagnostics.add(issues.slice(0, 1), [-2, 3]);
    assert.equal(count.value, "1");
  } finally {
    globalThis.document = previous;
  }
});

test("packet JSON preserves column scope, source counts and entity ids", () => {
  const issues = [{ scope: "column", origin: "source", position: [16, 0, 32], blockstate: "example:state []", reason: "unsupported_block_entity", count: 12, blockEntityId: "example:entity" }];
  const encoded = new TextEncoder().encode(JSON.stringify(issues));
  const packet = new ArrayBuffer(20 + encoded.length);
  new Uint8Array(packet).set(new TextEncoder().encode("NVWC"));
  const view = new DataView(packet);
  view.setUint32(4, 3, true);
  view.setUint32(16, encoded.length, true);
  new Uint8Array(packet, 20).set(encoded);
  const decoded = decodeColumn(packet);
  assert.deepEqual(decoded.issues, issues);
  assert.equal(decoded.unknown, 0);
});
