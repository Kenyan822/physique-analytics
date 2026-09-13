import { describe, expect, it } from "vitest";

import type { MealEstimate } from "@/lib/api/client";

import { EMPTY_DRAFT, confidenceLabel, toDraft } from "./estimate";

function estimate(over: Partial<MealEstimate> = {}): MealEstimate {
  return {
    name: "鶏むね肉のソテー",
    qty: "200g",
    kcal: 330,
    proteinG: 62,
    fatG: 7,
    carbG: 0,
    confidence: "medium",
    note: "皮なしとして計算",
    source: "ai_estimated",
    ...over,
  };
}

describe("toDraft", () => {
  it("推定結果を編集できる下書きにする", () => {
    const got = toDraft(estimate());

    expect(got.name).toBe("鶏むね肉のソテー");
    expect(got.qty).toBe("200g");
    expect(got.kcal).toBe("330");
    expect(got.proteinG).toBe("62");
  });

  it("source は ai_estimated になる", () => {
    // 推定値の比率が高い週は分析の確度を下げて扱う（docs/02-データモデル.md）
    expect(toDraft(estimate()).source).toBe("ai_estimated");
  });

  it("推定できなかった値は空欄にする", () => {
    // **0 で埋めない。** 0 kcal として記録され、実績が過小になる
    const got = toDraft(estimate({ kcal: null, fatG: null }));

    expect(got.kcal).toBe("");
    expect(got.fatG).toBe("");
  });

  it("0 は 0 のまま残す", () => {
    expect(toDraft(estimate({ carbG: 0 })).carbG).toBe("0");
  });

  it("qty が無ければ空欄", () => {
    expect(toDraft(estimate({ qty: null })).qty).toBe("");
  });
});

describe("EMPTY_DRAFT", () => {
  it("手入力として始まる", () => {
    expect(EMPTY_DRAFT.source).toBe("manual");
  });
});

describe("confidenceLabel", () => {
  it.each([
    ["low", "確信度 低"],
    ["medium", "確信度 中"],
    ["high", "確信度 高"],
  ])("%s", (v, want) => {
    expect(confidenceLabel(v)).toBe(want);
  });

  it("未知の値はそのまま出す（握りつぶさない）", () => {
    expect(confidenceLabel("なんか変な値")).toBe("確信度 なんか変な値");
  });

  it("無ければ null", () => {
    expect(confidenceLabel(null)).toBeNull();
  });
});
