import { describe, expect, it } from "vitest";

import { kcalFromMacros, toManualTargets } from "./manualTargets";

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
