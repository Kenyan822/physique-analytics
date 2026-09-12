import { describe, expect, it } from "vitest";

import { formatRemaining, nextRestSeconds } from "./rest";

describe("nextRestSeconds", () => {
  // 種目ごとの既定値（要件 T-05）。多関節は回復に時間が要る
  it("種目の defaultRestSec を使う", () => {
    expect(nextRestSeconds({ defaultRestSec: 180, isCompound: true })).toBe(180);
  });

  it("未設定なら多関節は長め", () => {
    expect(nextRestSeconds({ isCompound: true })).toBe(180);
    expect(nextRestSeconds({ isCompound: false })).toBe(90);
  });

  it("0 は「休憩なし」として尊重する", () => {
    expect(nextRestSeconds({ defaultRestSec: 0, isCompound: true })).toBe(0);
  });

  it("負の値は 0 に丸める", () => {
    expect(nextRestSeconds({ defaultRestSec: -10, isCompound: false })).toBe(0);
  });
});

describe("formatRemaining", () => {
  it("分:秒で表示する", () => {
    expect(formatRemaining(180)).toBe("3:00");
    expect(formatRemaining(95)).toBe("1:35");
    expect(formatRemaining(9)).toBe("0:09");
    expect(formatRemaining(0)).toBe("0:00");
  });

  // 経過しすぎてもマイナス表示にしない
  it("負の値は 0:00", () => {
    expect(formatRemaining(-5)).toBe("0:00");
  });
});
