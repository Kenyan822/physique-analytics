import Link from "next/link";

import { serverApi } from "@/lib/api/server";
import { tolerate } from "@/lib/api/tolerate";
import { formatJstDate, todayJst } from "@/lib/jst";

import { MealForm } from "./MealForm";
import {
  applyMealSet,
  copyMeals,
  createMeal,
  createMealSetFrom,
  deleteMeal,
  estimateMeal,
  loadSuggestions,
} from "./actions";

export const metadata = { title: "食事 | physique" };

export default async function MealsPage() {
  const date = todayJst();
  const yesterday = shiftDays(date, -1);
  const api = serverApi();
  const [{ items }, targets, { items: mealSets }] = await Promise.all([
    api.listMeals({ from: date, to: date }),
    // **422 で画面を落とさない。** フェーズ未登録は「まだ目標を出せない」であって
    // 異常ではない。ここで throw すると記録そのものができなくなる
    tolerate(() => api.dailyTargets(date), [422]),
    api.listMealSets(),
  ]);

  return (
    <main className="mx-auto w-full max-w-md lg:max-w-5xl">
      <header className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-line bg-bg/95 px-4 py-3 backdrop-blur lg:px-6 lg:py-4">
        <h1 className="text-lg font-semibold lg:text-xl">食事 {formatJstDate(date)}</h1>
        <Link
          href="/"
          className="pressable rounded-full border border-line px-3 py-1.5 text-sm text-muted"
        >
          今日の記録
        </Link>
      </header>

      <div className="pt-4">
        <MealForm
          date={date}
          yesterday={yesterday}
          recorded={items}
          targets={targets.value}
          targetsUnavailable={targets.unavailable}
          mealSets={mealSets}
          loadSuggestions={loadSuggestions}
          createMeal={createMeal}
          estimateMeal={estimateMeal}
          deleteMeal={deleteMeal}
          copyMeals={copyMeals}
          applyMealSet={applyMealSet}
          createMealSetFrom={createMealSetFrom}
        />
      </div>
    </main>
  );
}

/** YYYY-MM-DD を n 日ずらす。JST 固定なので UTC で計算してよい（ADR-0013） */
function shiftDays(date: string, n: number): string {
  const [y, m, d] = date.split("-").map(Number);

  return new Date(Date.UTC(y, m - 1, d + n)).toISOString().slice(0, 10);
}
