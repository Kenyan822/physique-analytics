import type { Meal, MealSlot } from "@/lib/api/client";

/** 1日の合計（要件 N-05 の実績側）。 */
export type Totals = {
  kcal: number;
  proteinG: number;
  fatG: number;
  carbG: number;
};

export const SLOTS: MealSlot[] = ["朝食", "昼食", "夕食", "間食"];

/**
 * 合計を出す。**未入力（null）は 0 として足す。**
 *
 * 「記録したが PFC は分からない」ものがあると合計は過少になるが、
 * 0 を入れて辻褄を合わせるより、欠けていることが分かる方がよい
 * （記録漏れの判定は分析側が持つ）。
 */
export function sumMeals(meals: Meal[]): Totals {
  return meals.reduce<Totals>(
    (acc, m) => ({
      kcal: acc.kcal + (m.kcal ?? 0),
      proteinG: acc.proteinG + (m.proteinG ?? 0),
      fatG: acc.fatG + (m.fatG ?? 0),
      carbG: acc.carbG + (m.carbG ?? 0),
    }),
    { kcal: 0, proteinG: 0, fatG: 0, carbG: 0 },
  );
}

/** PFC が1つも入っていない記録の件数。合計の確からしさを示すのに使う。 */
export function countWithoutMacros(meals: Meal[]): number {
  return meals.filter(
    (m) => m.kcal == null && m.proteinG == null && m.fatG == null && m.carbG == null,
  ).length;
}

/** slot ごとにまとめる。slot 未指定はまとめて最後に置く。 */
export function groupBySlot(meals: Meal[]): { slot: MealSlot | null; meals: Meal[] }[] {
  const out: { slot: MealSlot | null; meals: Meal[] }[] = [];

  for (const slot of SLOTS) {
    const items = meals.filter((m) => m.slot === slot);
    if (items.length > 0) out.push({ slot, meals: items });
  }

  const rest = meals.filter((m) => m.slot == null);
  if (rest.length > 0) out.push({ slot: null, meals: rest });

  return out;
}
