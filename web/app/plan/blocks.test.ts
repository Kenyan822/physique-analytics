import { describe, expect, it } from "vitest";

import type { PlanBlock } from "@/lib/api/client";

import { findBlockError, readSigned, totalMonths } from "./blocks";

function block(over: Partial<PlanBlock> = {}): PlanBlock {
  return { name: "Y1 増量", months: 6, lbmDeltaKgPerMonth: 0.3, bodyfatPctEnd: 15, ...over };
}

describe("findBlockError", () => {
  it("問題なければ null", () => {
    expect(findBlockError([block()])).toBeNull();
  });

  it("名前が空", () => {
    expect(findBlockError([block({ name: "  " })])?.index).toBe(0);
  });

  it("月数が範囲外", () => {
    expect(findBlockError([block({ months: 0 })])).not.toBeNull();
    expect(findBlockError([block({ months: 61 })])).not.toBeNull();
    expect(findBlockError([block({ months: 60 })])).toBeNull();
  });

  it("LBM の増減が ±2kg/月 を超える", () => {
    expect(findBlockError([block({ lbmDeltaKgPerMonth: 2.1 })])).not.toBeNull();
    expect(findBlockError([block({ lbmDeltaKgPerMonth: -2.1 })])).not.toBeNull();
    // 減量ブロックは負になる
    expect(findBlockError([block({ lbmDeltaKgPerMonth: -0.1 })])).toBeNull();
  });

  it("終了時点の体脂肪率が範囲外", () => {
    expect(findBlockError([block({ bodyfatPctEnd: 0 })])).not.toBeNull();
    expect(findBlockError([block({ bodyfatPctEnd: 60 })])).not.toBeNull();
    expect(findBlockError([block({ bodyfatPctEnd: 5 })])).toBeNull();
  });

  it("2件目の問題も見つける", () => {
    const got = findBlockError([block(), block({ months: 0 })]);

    expect(got?.index).toBe(1);
  });

  it("空なら null（保存できないことは別で止める）", () => {
    expect(findBlockError([])).toBeNull();
  });
});

describe("totalMonths", () => {
  it("月数を合計する", () => {
    expect(totalMonths([block({ months: 6 }), block({ months: 3 })])).toBe(9);
  });

  it("空なら 0", () => {
    expect(totalMonths([])).toBe(0);
  });
});

describe("readSigned", () => {
  it("負の値を読む（減量ブロックは負になる）", () => {
    expect(readSigned("-0.4", 0)).toBe(-0.4);
  });

  it("正の値をそのまま読む", () => {
    expect(readSigned("0.3", 0)).toBe(0.3);
  });

  it("全角で打たれた数字も読む", () => {
    expect(readSigned("－０.４", 0)).toBe(-0.4);
  });

  it("読めなければ直前の値に戻す", () => {
    expect(readSigned("abc", 0.3)).toBe(0.3);
    expect(readSigned("-", 0.3)).toBe(0.3);
  });

  it("マイナスだけを打った直前の値が負でも符号を二重にしない", () => {
    expect(readSigned("-", -0.4)).toBe(-0.4);
  });

  it("空欄は直前の値", () => {
    expect(readSigned("", -0.4)).toBe(-0.4);
  });
});
