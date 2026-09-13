import Link from "next/link";

import { serverApi } from "@/lib/api/server";
import { formatJstDate, todayJst } from "@/lib/jst";

import { LogForm } from "./LogForm";
import { deleteSet, loadLastPerformance, recordSet } from "./actions";

export const metadata = { title: "記録 | physique" };

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
    id: s.id,
    exerciseId: s.exerciseId,
    setNo: s.setNo,
    weightKg: s.weightKg,
    reps: s.reps,
    rir: s.rir ?? null,
    // サーバから読んだ時点で同期済み
    synced: true,
  }));

  return (
    <main className="mx-auto w-full max-w-md lg:max-w-5xl">
      {/* スクロールしても日付と戻り先が見えるようにする */}
      <header className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-line bg-bg/95 px-4 py-3 backdrop-blur lg:px-6 lg:py-4">
        <h1 className="text-lg font-semibold lg:text-xl">記録 {formatJstDate(date)}</h1>
        <Link
          href="/"
          className="pressable rounded-full border border-line px-3 py-1.5 text-sm text-muted"
        >
          今日の記録
        </Link>
      </header>

      <div className="pt-4">
        <LogForm
          exercises={exercises}
          templates={templates}
          recorded={recorded}
          loadLast={loadLastPerformance}
          recordSet={recordSet}
          deleteSet={deleteSet}
          date={date}
        />
      </div>
    </main>
  );
}
