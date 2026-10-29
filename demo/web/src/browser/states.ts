import type { Binding } from "./api";

export function propertyChoices(
  states: Binding[],
  current: Binding,
): Record<string, Binding[]> {
  // 只改一个属性，其他属性保留；选项直接携带全集中的 hash。
  const properties = Object.entries(current.java.properties);
  return Object.fromEntries(
    properties.map(([name]) => [
      name,
      states
        .filter((binding) =>
          properties.every(
            ([key, value]) =>
              key === name || binding.java.properties[key] === value,
          ),
        )
        .sort((a, b) =>
          a.java.properties[name].localeCompare(b.java.properties[name], undefined, {
            numeric: true,
          }),
        ),
    ]),
  );
}
