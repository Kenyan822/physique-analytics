import { describe, expect, it } from "vitest";

import type { Meal } from "@/lib/api/client";

import { countWithoutMacros, groupBySlot, sumMeals } from "./totals";

function meal(m: Partial<Meal>): Meal {
  return {
    id: crypto.randomUUID(),
    date: "2033-01-05",
    name: "x",
    source: "manual",
    createdAt: "2033-01-05T00:00:00+09:00",
    updatedAt: "2033-01-05T00:00:00+09:00",
    ...m,
  } as Meal;
}

describe("sumMeals", () => {
  it("合計する", () => {
    const got = sumMeals([
      meal({ kcal: 114, proteinG: 24.1, fatG: 1.5, carbG: 0.3 }),
      meal({ kcal: 180, proteinG: 6, fatG: 3, carbG: 31 }),
    ]);

    expect(got).toEqual({ kcal: 294, proteinG: 30.1, fatG: 4.5, carbG: 31.3 });
  });

  it("未入力は0として足す", () => {
    const got = sumMeals([meal({ kcal: 100 }), meal({})]);

    expect(got.kcal).toBe(100);
    expect(got.proteinG).toBe(0);
  });

  it("空なら0", () => {
    expect(sumMeals([])).toEqual({ kcal: 0, proteinG: 0, fatG: 0, carbG: 0 });
  });
});

describe("countWithoutMacros", () => {
  it("PFCが1つも無い記録を数える", () => {
    // 合計が過少なことを画面で示すために使う
    expect(countWithoutMacros([meal({ kcal: 100 }), meal({}), meal({ proteinG: 1 })])).toBe(1);
  });
});

describe("groupBySlot", () => {
  it("朝食・昼食・夕食・間食の順に並べる", () => {
    const got = groupBySlot([
      meal({ slot: "夕食", name: "a" }),
      meal({ slot: "朝食", name: "b" }),
      meal({ slot: "間食", name: "c" }),
    ]);

    expect(got.map((g) => g.slot)).toEqual(["朝食", "夕食", "間食"]);
  });

  it("記録の無い区分は出さない", () => {
    expect(groupBySlot([meal({ slot: "朝食" })]).map((g) => g.slot)).toEqual(["朝食"]);
  });

  it("slot未指定は最後にまとめる", () => {
    const got = groupBySlot([meal({}), meal({ slot: "朝食" })]);

    expect(got.map((g) => g.slot)).toEqual(["朝食", null]);
  });
});
