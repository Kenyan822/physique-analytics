"use client";

import { useCallback, useState, useTransition } from "react";

import type { Exercise, Template } from "@/lib/api/client";
import { useOfflineQueue } from "@/lib/offline/useOfflineQueue";

import { ExercisePicker } from "./ExercisePicker";
import { RestTimer } from "./RestTimer";
import { clampReps, clampWeight, SetInput, type SetValue } from "./SetInput";
import { nextRestSeconds } from "./rest";
import type { EditSetResult, LastPerformance, RecordSetResult } from "./actions";

type Props = {
  exercises: Exercise[];
  templates: Template[];
  /** 種目を選んだときに前回値を引く（要件 T-02） */
  loadLast: (exerciseId: string) => Promise<LastPerformance>;
  recordSet: (input: {
    id?: string;
    date: string;
    exerciseId: string;
    setNo: number;
    weightKg: number;
    reps: number;
    rir: number | null;
  }) => Promise<RecordSetResult>;
  /** 記録したセットを消す（要件 T-10） */
  deleteSet: (setId: string) => Promise<EditSetResult>;
  date: string;
  /** その日すでに記録してあるセット。画面を開き直しても続きから入力できるようにする */
  recorded: Logged[];
};

export type Logged = {
  /** クライアント生成の UUID。削除・修正に使う（要件 T-10） */
  id: string;
  exerciseId: string;
  setNo: number;
  weightKg: number;
  reps: number;
  rir: number | null;
  /** サーバに届いているか。未送信のセットは直せない */
  synced: boolean;
};

const EMPTY: SetValue = { weightKg: 20, reps: 8, rir: 2 };

export function LogForm({
  exercises,
  templates,
  loadLast,
  recordSet,
  deleteSet,
  date,
  recorded,
}: Props) {
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
      id: string;
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
  // 1日4種目という前提なので、49種目から毎回探す必要がない
  const choices = template
    ? template.items
        .map((it) => byId.get(it.exerciseId))
        .filter((e): e is Exercise => e !== undefined)
    : exercises;

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
            ? {
                weightKg: clampWeight(top.weightKg),
                reps: clampReps(top.reps),
                rir: top.rir ?? null,
              }
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
    // **ID はクライアントで振る。** 再送が冪等になり（要件 T-07）、
    // 送信を待たずに削除・修正の対象を特定できる（要件 T-10）
    const id = crypto.randomUUID();

    startTransition(async () => {
      // **積んだ時点で記録は確定**。送信の成否は表示で伝えるだけにする。
      // 「失敗したら入力し直し」では電波の悪いジムで使えない
      const res = await record({
        id,
        date,
        exerciseId,
        setNo,
        ...value,
        queuedAt: Date.now(),
      });

      // 記録したら次のセットの入力欄をそのまま開いておく（要件 T-04）。
      // 値は据え置き。次セットも同じ重量で入ることが多い
      setLogged((prev) => [...prev, { id, exerciseId, setNo, ...value, synced: res.synced }]);
      setRestStartedAt(Date.now());
      setError(res.synced ? null : (res.message ?? null));
    });
  }

  function removeSet(s: Logged) {
    setError(null);
    startTransition(async () => {
      const res = await deleteSet(s.id);
      if (!res.ok) {
        setError(res.message);

        return;
      }
      // **番号は振り直さない。** 消したセットの番号が空くだけにする。
      // 振り直すと、既に記録した番号と衝突して入力できなくなる
      setLogged((prev) => prev.filter((x) => x.id !== s.id));
    });
  }

  const restSec = exercise ? nextRestSeconds(exercise) : 0;
  const todayTonnage = logged.reduce((sum, l) => sum + l.weightKg * l.reps, 0);

  return (
    /*
     * 画面が広いときは「入力」と「今日の記録」を横に並べる。
     * 入力側を 28rem で止めているのは、ボタンや数字が広がると
     * ジムで使う縦画面とサイズ感がずれて、目測で押せなくなるため
     */
    <div className="px-4 pb-40 lg:grid lg:grid-cols-[28rem_minmax(0,1fr)] lg:items-start lg:gap-8 lg:px-6 lg:pb-12">
      <div className="flex flex-col gap-4">
        {(!online || pendingCount > 0) && (
          <p className="rounded-xl border border-warn/40 bg-warn/10 px-3 py-2 text-sm text-warn">
            {online
              ? `未送信 ${pendingCount} 件。再送を待っている`
              : `オフライン。記録は端末に残る（未送信 ${pendingCount} 件）`}
          </p>
        )}

        {templates.length > 0 && (
          <div className="flex gap-2 overflow-x-auto">
            <MenuChip active={templateId === ""} onClick={() => setTemplateId("")}>
              全種目
            </MenuChip>
            {templates.map((t) => (
              <MenuChip
                key={t.id}
                active={templateId === t.id}
                onClick={() => {
                  setTemplateId(templateId === t.id ? "" : t.id);
                  setExerciseId("");
                  setLast(null);
                }}
              >
                {t.name}
              </MenuChip>
            ))}
          </div>
        )}

        <ExercisePicker
          exercises={choices}
          selected={exercise}
          doneCount={doneCount}
          onSelect={selectExercise}
        />

        {exercise && <LastPerformanceView last={last} pending={pending} />}
        {exercise && <SetInput value={value} onChange={setValue} disabled={pending} />}
        {exercise && <RestTimer seconds={restSec} startedAt={restStartedAt} />}

        {error && (
          <p
            role="alert"
            className="rounded-xl border border-danger/40 bg-danger/10 px-3 py-2 text-sm text-danger"
          >
            {error}
          </p>
        )}

        {/* 狭い画面では親指が届く位置に固定される（.action-bar） */}
        {exercise && (
          <div className="action-bar">
            <button
              type="button"
              onClick={submit}
              disabled={pending}
              className="pressable h-16 w-full rounded-2xl bg-accent text-lg font-bold text-accent-ink disabled:opacity-50"
            >
              {pending ? "記録中…" : `${nextSetNo(exerciseId)} セット目を記録`}
            </button>
          </div>
        )}
      </div>

      {logged.length > 0 && (
        <section className="mt-4 rounded-2xl border border-line bg-surface p-4 lg:sticky lg:top-24 lg:mt-0">
          <div className="mb-3 flex items-baseline justify-between">
            <h2 className="text-sm font-medium">今日の記録</h2>
            <span className="tnum text-xs text-muted">
              {logged.length} セット / {Math.round(todayTonnage).toLocaleString()} kg
            </span>
          </div>
          <ul className="flex flex-col gap-1.5">
            {logged.map((s) => (
              <li
                key={`${s.exerciseId}-${s.setNo}`}
                className="flex items-baseline justify-between gap-3 text-sm"
              >
                <span className="min-w-0 truncate text-muted">
                  {byId.get(s.exerciseId)?.name ?? "?"}
                  <span className="tnum ml-1.5 text-xs">#{s.setNo}</span>
                </span>
                <span className="flex shrink-0 items-baseline gap-2">
                  <span className="tnum">
                    {s.weightKg}kg × {s.reps}
                    {s.rir !== null && <span className="text-muted"> @{s.rir}</span>}
                  </span>
                  <button
                    type="button"
                    onClick={() => removeSet(s)}
                    disabled={pending || !s.synced}
                    aria-label={`${byId.get(s.exerciseId)?.name ?? ""} ${s.setNo}セット目を削除`}
                    title={s.synced ? "削除" : "送信が終われば消せる"}
                    className="pressable rounded-lg px-1.5 text-muted disabled:opacity-30"
                  >
                    ×
                  </button>
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}

function MenuChip({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={`pressable shrink-0 rounded-full border px-3.5 py-2 text-sm ${
        active
          ? "border-accent bg-accent font-semibold text-accent-ink"
          : "border-line bg-surface text-muted"
      }`}
    >
      {children}
    </button>
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
    return (
      <div className="rounded-2xl border border-line bg-surface p-4 text-sm text-muted">
        前回値を取得中…
      </div>
    );
  }
  if (!last) return null;

  if (!last.date) {
    return (
      <div className="rounded-2xl border border-line bg-surface p-4 text-sm text-muted">
        この種目は初回。記録が貯まると前回値が出る
      </div>
    );
  }

  return (
    <section className="rounded-2xl border border-line bg-surface p-4">
      <div className="mb-2 flex items-baseline justify-between">
        <span className="text-xs text-muted">前回 {last.date}</span>
        {last.estimatedOneRm != null && (
          <span className="tnum text-xs text-muted">
            推定1RM <span className="text-ink">{last.estimatedOneRm.toFixed(1)}</span> kg
          </span>
        )}
      </div>
      <ul className="tnum flex flex-wrap gap-x-2 gap-y-1.5 text-sm">
        {(last.sets ?? []).map((s) => (
          <li key={s.id} className="rounded-lg bg-surface-2 px-2 py-1">
            {s.weightKg}×{s.reps}
            {s.rir != null && <span className="text-muted">@{s.rir}</span>}
          </li>
        ))}
      </ul>
    </section>
  );
}
