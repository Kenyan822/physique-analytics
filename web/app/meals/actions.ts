"use server";

import { revalidatePath } from "next/cache";

import {
  ApiError,
  type Meal,
  type MealInput,
  type MealSetInput,
  type MealSlot,
} from "@/lib/api/client";
import { serverApi } from "@/lib/api/server";

export type MealResult = { ok: true; meal: Meal } | { ok: false; message: string };
export type CopyResult = { ok: true; count: number } | { ok: false; message: string };

/** 食事を1件記録する（要件 N-01）。 */
export async function createMeal(input: MealInput): Promise<MealResult> {
  try {
    const meal = await serverApi().createMeal(input);
    revalidatePath("/meals");

    return { ok: true, meal };
  } catch (e) {
    return { ok: false, message: reason(e, "記録できなかった") };
  }
}

/** 食事を削除する。 */
export async function deleteMeal(id: string): Promise<{ ok: boolean; message?: string }> {
  try {
    await serverApi().deleteMeal(id);
    revalidatePath("/meals");

    return { ok: true };
  } catch (e) {
    return { ok: false, message: reason(e, "削除できなかった") };
  }
}

/** 過去の記録から候補を引く（要件 N-02）。 */
export async function loadSuggestions(q: string) {
  const { items } = await serverApi().mealSuggestions({ q, limit: 20 });

  return items;
}

/** 別の日の食事を写す（要件 N-04）。 */
export async function copyMeals(
  fromDate: string,
  toDate: string,
  slot?: MealSlot,
): Promise<CopyResult> {
  try {
    const { items } = await serverApi().copyMeals({ fromDate, toDate, slot });
    revalidatePath("/meals");

    return { ok: true, count: items.length };
  } catch (e) {
    return { ok: false, message: reason(e, "複製できなかった") };
  }
}

function reason(e: unknown, fallback: string): string {
  if (e instanceof ApiError) {
    // API が返した理由をそのまま見せる。何が悪いか分からないと直せない
    return e.problem?.detail ?? e.problem?.title ?? e.message;
  }

  return e instanceof Error ? e.message : fallback;
}

/** 食事セットをその日に展開する（要件 N-03）。 */
export async function applyMealSet(id: string, date: string, slot?: MealSlot): Promise<CopyResult> {
  try {
    const { items } = await serverApi().applyMealSet(id, { date, slot });
    revalidatePath("/meals");

    return { ok: true, count: items.length };
  } catch (e) {
    return { ok: false, message: reason(e, "展開できなかった") };
  }
}

/** 今日の記録から食事セットを作る（要件 N-03）。 */
export async function createMealSetFrom(
  name: string,
  meals: Meal[],
  slot?: MealSlot,
): Promise<{ ok: boolean; message?: string }> {
  const input: MealSetInput = {
    name,
    slot,
    items: meals.map((m) => ({
      name: m.name,
      qty: m.qty ?? null,
      kcal: m.kcal ?? null,
      proteinG: m.proteinG ?? null,
      fatG: m.fatG ?? null,
      carbG: m.carbG ?? null,
    })),
  };

  try {
    await serverApi().createMealSet(input);
    revalidatePath("/meals");

    return { ok: true };
  } catch (e) {
    return { ok: false, message: reason(e, "セットを作れなかった") };
  }
}
