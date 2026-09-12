import Link from "next/link";

import { serverApi } from "@/lib/api/server";
import { formatJstDate, todayJst } from "@/lib/jst";

import { LogForm } from "./LogForm";
import { loadLastPerformance, recordSet } from "./actions";

export const metadata = { title: "記録 | physique-analytics" };

export default async function LogPage() {
  const date = todayJst();
  const api = serverApi();

  const [{ items: exercises }, { items: templates }, { items: sessions }] = await Promise.all([
    api.listExercises({}),
    api.listTemplates(),
    // **その日の記録を先に読む。** 画面を閉じて開き直したときに
    // セット番号が1に戻ると、既に記録した番号と衝突して入力できない
    api.listWorkoutSessions({ from: date, to: date, limit: 1 }),
  ]);

  const recorded = (sessions[0]?.sets ?? []).map((s) => ({
    exerciseId: s.exerciseId,
    setNo: s.setNo,
    weightKg: s.weightKg,
    reps: s.reps,
    rir: s.rir ?? null,
  }));

  return (
    <main className="mx-auto flex max-w-md flex-col gap-6 p-4">
      <header className="flex items-baseline justify-between">
        <h1 className="text-xl font-semibold">記録 {formatJstDate(date)}</h1>
        <Link href="/" className="text-sm text-gray-600 underline dark:text-gray-400">
          今日の記録
        </Link>
      </header>

      <LogForm
        exercises={exercises}
        templates={templates}
        recorded={recorded}
        loadLast={loadLastPerformance}
        recordSet={recordSet}
        date={date}
      />
    </main>
  );
}
