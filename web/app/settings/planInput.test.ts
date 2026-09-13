import { describe, expect, it } from "vitest";

import type { Plan } from "@/lib/api/client";

import { isMonth, toPlanInput, type PlanFormValues } from "./planInput";

const plan: Plan = {
  heightCm: 175,
  startDate: "2026-09-01",
  baselineWeightKg: 75,
  baselineBodyfatPct: 20,
  baselineMonth: "2026-09",
  phases: [],
  nutrition: {
    cut: { proteinGPerKg: 2.2, fatGPerKg: 0.85 },
    deepCut: { proteinGPerKg: 2.4, fatGPerKg: 0.7 },
    bulk: { proteinGPerKg: 2, fatGPerKg: 1 },
    deepCutBfThreshold: 12,
    carbMinG: 100,
  },
  volumeRanges: [],
};

function form(over: Partial<PlanFormValues> = {}): PlanFormValues {
  return {
    heightCm: "175",
    baselineWeightKg: "75",
    baselineBodyfatPct: "20",
    baselineMonth: "2026-09",
    phases: [],
    nutrition: plan.nutrition,
    volumeRanges: [],
    ...over,
  };
}

describe("toPlanInput", () => {
  it("起点を落とさない", () => {
    // PUT /v1/plan はまるごと置き換わる。送らない項目は null になる
    const got = toPlanInput(plan, form());

    expect(got.baselineWeightKg).toBe(75);
    expect(got.baselineBodyfatPct).toBe(20);
    expect(got.baselineMonth).toBe("2026-09");
  });

  it("空欄は null にする", () => {
    const got = toPlanInput(
      plan,
      form({ heightCm: "", baselineWeightKg: "", baselineBodyfatPct: "", baselineMonth: "" }),
    );

    expect(got.heightCm).toBeNull();
    expect(got.baselineWeightKg).toBeNull();
    expect(got.baselineBodyfatPct).toBeNull();
    expect(got.baselineMonth).toBeNull();
  });

  it("全角で打たれた数字も読む", () => {
    const got = toPlanInput(plan, form({ baselineWeightKg: "７５．５" }));

    expect(got.baselineWeightKg).toBe(75.5);
  });

  it("開始日は計画のものを引き継ぐ", () => {
    expect(toPlanInput(plan, form()).startDate).toBe("2026-09-01");
  });
});

describe("isMonth", () => {
  it.each([
    ["2026-09", true],
    ["2026-12", true],
    ["2026-9", false],
    ["2026-13", false],
    ["2026-00", false],
    ["2026-09-01", false],
    ["", false],
  ])("%s → %s", (value, want) => {
    expect(isMonth(value)).toBe(want);
  });
});
