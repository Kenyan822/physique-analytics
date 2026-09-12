import Link from "next/link";

import { serverApi } from "@/lib/api/server";
import { formatJstDate, todayJst } from "@/lib/jst";

export default async function Home() {
  const date = todayJst();
  const api = serverApi();

  const [{ items: sessions }, { items: exercises }] = await Promise.all([
    api.listWorkoutSessions({ from: date, to: date, limit: 1 }),
    api.listExercises({}),
  ]);

  const today = sessions[0];
  const nameById = new Map(exercises.map((e) => [e.id, e.name]));

  // 種目ごとにまとめる。セットは setNo 順に並んでいる
  const byExercise = new Map<string, typeof today.sets>();
  for (const s of today?.sets ?? []) {
    const list = byExercise.get(s.exerciseId) ?? [];
    list.push(s);
    byExercise.set(s.exerciseId, list);
  }

  const totalTonnage = (today?.sets ?? []).reduce((sum, s) => sum + s.weightKg * s.reps, 0);

  return (
    <main className="mx-auto flex max-w-md flex-col gap-6 p-4">
      <header className="flex items-baseline justify-between">
        <h1 className="text-xl font-semibold">{formatJstDate(date)}</h1>
        <Link
          href="/log"
          className="rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white dark:bg-white dark:text-gray-900"
        >
          記録する
        </Link>
      </header>

      {byExercise.size === 0 ? (
        <p className="text-sm text-gray-500">今日はまだ記録が無い。</p>
      ) : (
        <>
          <p className="text-sm text-gray-600 tabular-nums dark:text-gray-400">
            {byExercise.size} 種目 / {today.sets.length} セット / トン数{" "}
            {Math.round(totalTonnage).toLocaleString()} kg
          </p>

          <ul className="flex flex-col gap-4">
            {[...byExercise].map(([exerciseId, sets]) => (
              <li key={exerciseId}>
                <h2 className="mb-1 font-medium">{nameById.get(exerciseId) ?? "（不明な種目）"}</h2>
                <ul className="flex flex-wrap gap-x-3 gap-y-1 text-sm tabular-nums text-gray-700 dark:text-gray-300">
                  {sets.map((s) => (
                    <li key={s.id}>
                      {s.weightKg}kg×{s.reps}
                      {s.rir != null && `@${s.rir}`}
                    </li>
                  ))}
                </ul>
              </li>
            ))}
          </ul>
        </>
      )}
    </main>
  );
}
