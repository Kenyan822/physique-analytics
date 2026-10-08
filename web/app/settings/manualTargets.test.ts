import { describe, expect, it } from "vitest";

import type { ManualTargetEntry } from "@/lib/api/client";

import { entryStatuses, kcalFromMacros, toManualTargets } from "./manualTargets";

describe("PFC から kcal を出す", () => {
  it("Atwater 係数 4/9/4", () => {
    // サーバの analytics.KcalFromMacros と一致させる
    expect(kcalFromMacros(30, 10, 60)).toBe(450);
    expect(kcalFromMacros(180, 70, 250)).toBe(2350);
  });

  it("端数は四捨五入", () => {
    // 120.4 + 91.8 + 241.2 = 453.4
    expect(kcalFromMacros(30.1, 10.2, 60.3)).toBe(453);
  });

  it("全部 0", () => {
    expect(kcalFromMacros(0, 0, 0)).toBe(0);
  });
});

describe("入力を目標にする", () => {
  it("3つ揃っていれば通る", () => {
    expect(toManualTargets({ proteinG: "180", fatG: "70", carbG: "250" })).toEqual({
      proteinG: 180,
      fatG: 70,
      carbG: 250,
    });
  });

  it("前後の空白を無視する", () => {
    expect(toManualTargets({ proteinG: " 180 ", fatG: "70", carbG: "250" })?.proteinG).toBe(180);
  });

  it("小数を読む", () => {
    expect(toManualTargets({ proteinG: "180.5", fatG: "70", carbG: "250" })?.proteinG).toBe(180.5);
  });

  // **3つで1組。** 1つ欠けた目標は意味を成さない（iOS と同じ規則）
  it("**1つでも空なら null**", () => {
    expect(toManualTargets({ proteinG: "180", fatG: "", carbG: "250" })).toBeNull();
    expect(toManualTargets({ proteinG: "", fatG: "", carbG: "" })).toBeNull();
  });

  it("読めない値は null", () => {
    expect(toManualTargets({ proteinG: "だいたい180", fatG: "70", carbG: "250" })).toBeNull();
  });

  it("負は受け付けない", () => {
    expect(toManualTargets({ proteinG: "-1", fatG: "70", carbG: "250" })).toBeNull();
  });

  it("上限を超えたら null", () => {
    expect(toManualTargets({ proteinG: "1001", fatG: "70", carbG: "250" })).toBeNull();
    expect(toManualTargets({ proteinG: "180", fatG: "70", carbG: "2001" })).toBeNull();
  });
});

describe("履歴の状態", () => {
  const entry = (id: string, startsOn: string): ManualTargetEntry => ({
    id,
    startsOn,
    proteinG: 180,
    fatG: 70,
    carbG: 250,
    kcal: 2350,
    createdAt: "2026-10-08T00:00:00+09:00",
  });
  // サーバは開始日の新しい順で返す
  const entries = [
    entry("future", "2026-11-01"),
    entry("now", "2026-10-08"),
    entry("old", "2026-10-01"),
  ];

  it("開始日が今日以前で最新の行が適用中", () => {
    expect(entryStatuses(entries, "2026-10-10")).toEqual({
      future: "scheduled",
      now: "current",
      old: "past",
    });
  });

  it("開始日が今日なら、その行が適用中", () => {
    expect(entryStatuses(entries, "2026-10-08").now).toBe("current");
  });

  it("全部未来なら適用中は無い（自動計算が使われる）", () => {
    expect(Object.values(entryStatuses([entry("a", "2026-12-01")], "2026-10-08"))).toEqual([
      "scheduled",
    ]);
  });

  it("履歴が空なら空", () => {
    expect(entryStatuses([], "2026-10-08")).toEqual({});
  });
});
