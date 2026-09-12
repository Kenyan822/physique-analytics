"use client";

import { useState, useTransition } from "react";

import type { Exercise } from "@/lib/api/client";

import { clampReps, clampWeight, SetInput, type SetValue } from "./SetInput";
import type { LastPerformance, RecordSetResult } from "./actions";

type Props = {
  exercises: Exercise[];
  /** 種目を選んだときに前回値を引く（要件 T-02） */
  loadLast: (exerciseId: string) => Promise<LastPerformance>;
  recordSet: (input: {
    date: string;
    exerciseId: string;
    setNo: number;
    weightKg: number;
    reps: number;
    rir: number | null;
  }) => Promise<RecordSetResult>;
  date: string;
};

type Logged = { setNo: number; weightKg: number; reps: number; rir: number | null };

const EMPTY: SetValue = { weightKg: 20, reps: 8, rir: 2 };

export function LogForm({ exercises, loadLast, recordSet, date }: Props) {
  const [exerciseId, setExerciseId] = useState("");
  const [value, setValue] = useState<SetValue>(EMPTY);
  const [last, setLast] = useState<LastPerformance | null>(null);
  const [logged, setLogged] = useState<Logged[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  const exercise = exercises.find((e) => e.id === exerciseId);

  function selectExercise(id: string) {
    setExerciseId(id);
    setLogged([]);
    setError(null);
    setLast(null);
    if (!id) return;

    startTransition(async () => {
      try {
        const res = await loadLast(id);
        setLast(res);
        // **前回値をそのまま初期値にする（要件 T-02）。**
        // ジムでの入力の大半は「前回と同じか少し増やす」なので、
        // これが合っていれば操作は1〜2タップで済む
        const top = res.sets?.[0];
        if (top) {
          setValue({
            weightKg: clampWeight(top.weightKg),
            reps: clampReps(top.reps),
            rir: top.rir ?? null,
          });
        } else {
          setValue(EMPTY);
        }
      } catch (e) {
        setError(e instanceof Error ? e.message : "前回値を取れなかった");
      }
    });
  }

  function submit() {
    if (!exerciseId) return;
    setError(null);

    const setNo = logged.length + 1;
    startTransition(async () => {
      const res = await recordSet({ date, exerciseId, setNo, ...value });
      if (!res.ok) {
        setError(res.message);
        return;
      }
      // 記録したら次のセットの入力欄をそのまま開いておく（要件 T-04）。
      // 値は据え置き。次セットも同じ重量で入ることが多い
      setLogged((prev) => [...prev, { setNo, ...value }]);
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <label className="flex flex-col gap-2">
        <span className="text-sm text-gray-600 dark:text-gray-400">種目</span>
        <select
          value={exerciseId}
          onChange={(e) => selectExercise(e.target.value)}
          className="h-12 rounded-lg border border-gray-300 bg-transparent px-3 text-base dark:border-gray-700"
        >
          <option value="">選ぶ</option>
          {exercises.map((e) => (
            <option key={e.id} value={e.id}>
              {e.name}（{e.muscleGroup}）
            </option>
          ))}
        </select>
      </label>

      {exercise && <LastPerformanceView last={last} pending={pending} />}

      {exercise && (
        <>
          <SetInput value={value} onChange={setValue} disabled={pending} />

          <button
            type="button"
            onClick={submit}
            disabled={pending}
            className="h-14 rounded-lg bg-gray-900 text-lg font-semibold text-white disabled:opacity-40 dark:bg-white dark:text-gray-900"
          >
            {pending ? "記録中…" : `${logged.length + 1} セット目を記録`}
          </button>
        </>
      )}

      {error && (
        <p role="alert" className="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950 dark:text-red-300">
          {error}
        </p>
      )}

      {logged.length > 0 && (
        <section>
          <h2 className="mb-2 text-sm text-gray-600 dark:text-gray-400">今日の記録</h2>
          <ul className="flex flex-col gap-1">
            {logged.map((s) => (
              <li key={s.setNo} className="tabular-nums text-sm">
                {s.setNo}セット目 {s.weightKg}kg × {s.reps}回
                {s.rir !== null && ` @RIR${s.rir}`}
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}

/** 前回の実施内容（要件 T-02 / T-09）。その場で超えられるか判断できるようにする。 */
function LastPerformanceView({
  last,
  pending,
}: {
  last: LastPerformance | null;
  pending: boolean;
}) {
  if (pending && !last) {
    return <p className="text-sm text-gray-500">前回値を取得中…</p>;
  }
  if (!last) return null;

  if (!last.date) {
    return <p className="text-sm text-gray-500">この種目は初回。記録が貯まると前回値が出る</p>;
  }

  return (
    <section className="rounded-lg bg-gray-100 p-3 text-sm dark:bg-gray-900">
      <div className="mb-1 flex items-baseline justify-between">
        <span className="text-gray-600 dark:text-gray-400">前回 {last.date}</span>
        {last.estimatedOneRm != null && (
          <span className="tabular-nums text-gray-600 dark:text-gray-400">
            推定1RM {last.estimatedOneRm.toFixed(1)}kg
          </span>
        )}
      </div>
      <ul className="flex flex-wrap gap-x-3 gap-y-1 tabular-nums">
        {(last.sets ?? []).map((s) => (
          <li key={s.id}>
            {s.weightKg}kg×{s.reps}
            {s.rir != null && `@${s.rir}`}
          </li>
        ))}
      </ul>
    </section>
  );
}
