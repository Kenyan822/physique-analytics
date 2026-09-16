import { SiteHeader } from "@/app/SiteHeader";
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
    <>
      {/* スクロールしても日付とメニューが見えるようにする */}
      <SiteHeader title="記録" subtitle={formatJstDate(date)} />

      <main className="mx-auto w-full max-w-6xl pt-4">
        <div>
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
    </>
  );
}
