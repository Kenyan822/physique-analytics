import { describe, expect, it } from "vitest";

import { clampReps, clampRir, clampWeight } from "./SetInput";

describe("clampWeight", () => {
  const cases: [string, number, number][] = [
    ["2.5 刻みでない値もそのまま通す（マシンやダンベルは刻みが違う）", 62, 62],
    ["+/- の加算で出る浮動小数点の誤差を消す", 0.1 + 0.2, 0.3],
    ["小数第2位まで残す（1.25kg のマイクロプレート）", 61.25, 61.25],
    // 二進で丸めると 1.005 * 100 が 100.49999… になって 1.00 に落ちる
    ["十進で丸める", 1.005, 1.01],
    ["指数表記になる極端な値でも壊れない", 1e21, 500],
    ["下限は0", -5, 0],
    ["上限は500", 999, 500],
  ];

  for (const [name, input, want] of cases) {
    it(name, () => {
      expect(clampWeight(input)).toBe(want);
    });
  }
});

describe("clampReps", () => {
  const cases: [string, number, number][] = [
    ["整数に丸める", 8.4, 8],
    ["四捨五入", 8.6, 9],
    ["下限は0", -1, 0],
    ["上限は100", 200, 100],
  ];

  for (const [name, input, want] of cases) {
    it(name, () => {
      expect(clampReps(input)).toBe(want);
    });
  }
});

describe("clampRir", () => {
  it("上限は10", () => {
    expect(clampRir(11)).toBe(10);
  });

  it("下限は0", () => {
    expect(clampRir(-1)).toBe(0);
  });
});
