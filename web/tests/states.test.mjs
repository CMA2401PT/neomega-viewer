import { test } from "node:test";
import assert from "node:assert/strict";
import { propertyChoices } from "../src/browser/states.ts";

function binding(id, properties) {
  return { id, java: { name: "minecraft:oak_stairs", properties } };
}

test("property changes select a catalog hash and retain every other property", () => {
  const states = [];
  for (const facing of ["east", "west"])
    for (const half of ["bottom", "top"])
      for (const shape of ["straight", "inner_left"])
        states.push(binding(100 + states.length, { facing, half, shape }));
  const current = states[0];
  const choices = propertyChoices(states, current);
  assert.deepEqual(choices.half.map((state) => state.java.properties.half), ["bottom", "top"]);
  const top = choices.half[1];
  assert.notEqual(top.id, current.id);
  assert.equal(top, states.find((state) => state.id === top.id));
  assert.deepEqual(top.java.properties, { facing: "east", half: "top", shape: "straight" });
  const west = propertyChoices(states, top).facing[1];
  assert.deepEqual(west.java.properties, { facing: "west", half: "top", shape: "straight" });
  assert.deepEqual(current.java.properties, { facing: "east", half: "bottom", shape: "straight" });
});

test("only available combinations are offered and numeric values stay ordered", () => {
  const states = [
    binding(1, { power: "0", north: "none" }),
    binding(2, { power: "15", north: "none" }),
    binding(3, { power: "2", north: "none" }),
    binding(4, { power: "15", north: "side" }),
  ];
  const choices = propertyChoices(states, states[0]);
  assert.deepEqual(choices.power.map((state) => state.id), [1, 3, 2]);
  assert.deepEqual(choices.north.map((state) => state.id), [1]);
  assert.deepEqual(propertyChoices([binding(5, {})], binding(5, {})), {});
});
