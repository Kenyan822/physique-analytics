"use client";

import { useCallback, useState, useTransition } from "react";

import { useOfflineQueue } from "@/lib/offline/useOfflineQueue";

import type { Exercise, Template } from "@/lib/api/client";

import { clampReps, clampWeight, SetInput, type SetValue } from "./SetInput";
import { RestTimer } from "./RestTimer";
import { nextRestSeconds } from "./rest";
import type { LastPerformance, RecordSetResult } from "./actions";

type Props = {
  exercises: Exercise[];
  templates: Template[];
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
  /** その日すでに記録してあるセット。画面を開き直しても続きから入力できるようにする */
  recorded: Logged[];
};

export type Logged = {
  exerciseId: string;
  setNo: number;
  weightKg: number;
  reps: number;
  rir: number | null;
};

const EMPTY: SetValue = { weightKg: 20, reps: 8, rir: 2 };

export function LogForm({ exercises, templates, loadLast, recordSet, date, recorded }: Props) {
  const [templateId, setTemplateId] = useState("");
  const [exerciseId, setExerciseId] = useState("");
  const [value, setValue] = useState<SetValue>(EMPTY);
  const [last, setLast] = useState<LastPerformance | null>(null);
  const [logged, setLogged] = useState<Logged[]>(recorded);
  const [restStartedAt, setRestStartedAt] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  // 記録はキューに積んでから送る（要件 T-07）。ジムは電波が悪い
  const send = useCallback(
    async (item: {
      date: string;
      exerciseId: string;
      setNo: number;
      weightKg: number;
      reps: number;
      rir: number | null;
    }) => {
      const res = await recordSet(item);

      return res.ok ? ({ ok: true } as const) : ({ ok: false, message: res.message } as const);
    },
    [recordSet],
  );
  const { record, pendingCount, online } = useOfflineQueue({ send });

  const byId = new Map(exercises.map((e) => [e.id, e]));
  const exercise = byId.get(exerciseId);
  const template = templates.find((t) => t.id === templateId);

  // テンプレートを選ぶと、その日の種目だけに絞る（要件 T-06）。
  // 1日4種目という前提（要件 T-05）なので、49種目から毎回探す必要がない
  const choices = template
    ? template.items
        .map((it) => byId.get(it.exerciseId))
        .filter((e): e is Exercise => e !== undefined)
    : exercises;

  /** その種目で今日すでに記録したセット数 */
  const doneCount = (id: string) => logged.filter((l) => l.exerciseId === id).length;

  /**
   * 次のセット番号。**件数ではなく最大値 + 1** にする。
   * 途中のセットを消したあとに件数で決めると、既存の番号と衝突する
   */
  const nextSetNo = (id: string) =>
    logged.filter((l) => l.exerciseId === id).reduce((max, l) => Math.max(max, l.setNo), 0) + 1;

  function selectExercise(id: string) {
    setExerciseId(id);
    setError(null);
    setLast(null);
    setRestStartedAt(null);
    if (!id) return;

    startTransition(async () => {
      try {
        const res = await loadLast(id);
        setLast(res);
        // **前回値をそのまま初期値にする（要件 T-02）。**
        // ジムでの入力の大半は「前回と同じか少し増やす」なので、
        // これが合っていれば操作は1〜2タップで済む
        const top = res.sets?.[0];
        setValue(
          top
            ? { weightKg: clampWeight(top.weightKg), reps: clampReps(top.reps), rir: top.rir ?? null }
            : EMPTY,
        );
      } catch (e) {
        setError(e instanceof Error ? e.message : "前回値を取れなかった");
      }
    });
  }

  function submit() {
    if (!exerciseId) return;
    setError(null);

    const setNo = nextSetNo(exerciseId);
    startTransition(async () => {
      // **積んだ時点で記録は確定**。送信の成否は表示で伝えるだけにする。
      // 「失敗したら入力し直し」では電波の悪いジムで使えない
      const res = await record({
        id: crypto.randomUUID(),
        date,
        exerciseId,
        setNo,
        ...value,
        queuedAt: Date.now(),
      });

      // 記録したら次のセットの入力欄をそのまま開いておく（要件 T-04）。
      // 値は据え置き。次セットも同じ重量で入ることが多い
      setLogged((prev) => [...prev, { exerciseId, setNo, ...value }]);
      setRestStartedAt(Date.now());
      setError(res.synced ? null : (res.message ?? null));
    });
  }

  const restSec = exercise ? nextRestSeconds(exercise) : 0;

  return (
    <div className="flex flex-col gap-6">
      {(!online || pendingCount > 0) && (
        <p className="rounded-lg bg-amber-100 p-3 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-200">
          {online
            ? `未送信 ${pendingCount} 件。再送を待っている`
            : `オフライン。記録は端末に残る（未送信 ${pendingCount} 件）`}
        </p>
      )}

      {templates.length > 0 && (
        <label className="flex flex-col gap-2">
          <span className="text-sm text-gray-600 dark:text-gray-400">メニュー</span>
          <select
            value={templateId}
            onChange={(e) => {
              setTemplateId(e.target.value);
              setExerciseId("");
              setLast(null);
            }}
            className="h-12 rounded-lg border border-gray-300 bg-transparent px-3 text-base dark:border-gray-700"
          >
            <option value="">指定しない（全種目）</option>
            {templates.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
              </option>
            ))}
          </select>
        </label>
      )}

      <label className="flex flex-col gap-2">
        <span className="text-sm text-gray-600 dark:text-gray-400">種目</span>
        <select
          value={exerciseId}
          onChange={(e) => selectExercise(e.target.value)}
          className="h-12 rounded-lg border border-gray-300 bg-transparent px-3 text-base dark:border-gray-700"
        >
          <option value="">選ぶ</option>
          {choices.map((e) => {
            const n = doneCount(e.id);

            return (
              <option key={e.id} value={e.id}>
                {e.name}（{e.muscleGroup}）{n > 0 ? ` ✓${n}` : ""}
              </option>
            );
          })}
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
            {pending ? "記録中…" : `${nextSetNo(exerciseId)} セット目を記録`}
          </button>

          <RestTimer seconds={restSec} startedAt={restStartedAt} />
        </>
      )}

      {error && (
        <p
          role="alert"
          className="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-950 dark:text-red-300"
        >
          {error}
        </p>
      )}

      {logged.length > 0 && (
        <section>
          <h2 className="mb-2 text-sm text-gray-600 dark:text-gray-400">今日の記録</h2>
          <ul className="flex flex-col gap-1">
            {logged.map((s) => (
              <li key={`${s.exerciseId}-${s.setNo}`} className="text-sm tabular-nums">
                {byId.get(s.exerciseId)?.name ?? "?"} {s.setNo}セット目 {s.weightKg}kg × {s.reps}回
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
function LastPerformanceView({ last, pending }: { last: LastPerformance | null; pending: boolean }) {
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
