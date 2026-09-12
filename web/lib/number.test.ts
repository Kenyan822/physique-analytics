import { describe, expect, it } from "vitest";

import { formatDiff, parseInput, round2 } from "./number";

describe("round2", () => {
  const cases: [string, number, number][] = [
    ["浮動小数点の誤差を消す", 0.1 + 0.2, 0.3],
    ["十進で丸める", 1.005, 1.01],
    ["指数表記になる値は丸めずに返す", 1e21, 1e21],
  ];

  for (const [name, input, want] of cases) {
    it(name, () => {
      expect(round2(input)).toBe(want);
    });
  }
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

describe("formatDiff", () => {
  const cases: [string, number | null, number | null | undefined, string | null][] = [
    ["増えたら + を付ける", 86.5, 86, "+0.5"],
    ["減ったら − を付ける", 85, 86, "−1"],
    ["同じなら出さない", 86, 86, null],
    ["前回が無ければ出さない", 86, null, null],
    ["前回が未設定でも出さない", 86, undefined, null],
    ["今回が無ければ出さない", null, 86, null],
    // 39.1 - 39 は 0.10000000000000142 になる
    ["誤差を出さない", 39.1, 39, "+0.1"],
  ];

  for (const [name, current, previous, want] of cases) {
    it(name, () => {
      expect(formatDiff(current, previous)).toBe(want);
    });
  }
});
