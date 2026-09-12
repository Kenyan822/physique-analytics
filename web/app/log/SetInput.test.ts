import { describe, expect, it } from "vitest";

import { clampReps, clampRir, clampWeight, parseInput } from "./SetInput";

describe("clampWeight", () => {
  const cases: [string, number, number][] = [
    ["2.5 刻みでない値もそのまま通す（マシンやダンベルは刻みが違う）", 62, 62],
    ["+/- の加算で出る浮動小数点の誤差を消す", 0.1 + 0.2, 0.3],
    ["小数第2位まで残す（1.25kg のマイクロプレート）", 61.25, 61.25],
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

describe("parseInput", () => {
  const cases: [string, string, number][] = [
    ["小数を読む", "62.5", 62.5],
    // 日本語入力のまま数字を打つと全角になる。弾かずに読む
    ["全角数字を読む", "６２．５", 62.5],
    ["前後の空白を無視する", " 8 ", 8],
    ["空文字は元の値に戻す", "", 40],
    ["数値でなければ元の値に戻す", "abc", 40],
    ["マイナスは読まない（0に落ちるのを防ぐ）", "-3", 40],
  ];

  for (const [name, input, want] of cases) {
    it(name, () => {
      expect(parseInput(input, 40)).toBe(want);
    });
  }
});
