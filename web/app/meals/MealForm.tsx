"use client";

import Link from "next/link";
import { useCallback, useState, useTransition } from "react";

import type {
  DailyTargets,
  Meal,
  MealSet,
  MealSlot,
  MealSource,
  MealSuggestion,
} from "@/lib/api/client";

import { EstimatePanel } from "./EstimatePanel";
import { MealPicker } from "./MealPicker";
import type { CopyResult, EstimateResult, MealResult } from "./actions";
import { EMPTY_DRAFT, type Draft } from "./estimate";
import { SLOTS, countWithoutMacros, groupBySlot, sumMeals } from "./totals";

type Props = {
  date: string;
  /** 前日の日付。1操作で写せるようにする（要件 N-04） */
  yesterday: string;
  /**
   * 区分の初期選択。**サーバで決めて渡す**（#175）。
   * ここで `new Date()` を見ると、SSR（UTC）とブラウザ（JST）で答えが割れて
   * hydration が食い違う
   */
  initialSlot: MealSlot;
  recorded: Meal[];
  /**
   * その日の摂取目標（要件 N-05）。TDEE を推定できないときは target が無い。
   * **フェーズ未登録だと目標そのものを出せない**ので null になる
   */
  targets: DailyTargets | null;
  /** 目標を出せない理由。出せていれば null */
  targetsUnavailable: string | null;
  /** 保存してある食事セット（要件 N-03） */
  mealSets: MealSet[];
  loadSuggestions: (q: string) => Promise<MealSuggestion[]>;
  createMeal: (input: {
    date: string;
    slot?: MealSlot;
    name: string;
    qty?: string | null;
    kcal?: number | null;
    proteinG?: number | null;
    fatG?: number | null;
    carbG?: number | null;
    source?: MealSource;
  }) => Promise<MealResult>;
  /** 写真から PFC を推定する（要件 N-06） */
  estimateMeal: (form: FormData) => Promise<EstimateResult>;
  deleteMeal: (id: string) => Promise<{ ok: boolean; message?: string }>;
  copyMeals: (fromDate: string, toDate: string) => Promise<CopyResult>;
  applyMealSet: (id: string, date: string, slot?: MealSlot) => Promise<CopyResult>;
  createMealSetFrom: (
    name: string,
    meals: Meal[],
    slot?: MealSlot,
  ) => Promise<{ ok: boolean; message?: string }>;
};

export function MealForm({
  date,
  yesterday,
  initialSlot,
  recorded,
  targets,
  targetsUnavailable,
  mealSets,
  loadSuggestions,
  createMeal,
  estimateMeal,
  deleteMeal,
  copyMeals,
  applyMealSet,
  createMealSetFrom,
}: Props) {
  const [meals, setMeals] = useState<Meal[]>(recorded);
  const [slot, setSlot] = useState<MealSlot>(initialSlot);
  const [draft, setDraft] = useState<Draft>(EMPTY_DRAFT);
  const [picking, setPicking] = useState(false);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [pending, startTransition] = useTransition();

  const totals = sumMeals(meals);
  const unknown = countWithoutMacros(meals);
  const groups = groupBySlot(meals);

  const pick = useCallback((s: MealSuggestion) => {
    // 直近の値をそのまま入れる。**選んだ時点で入力が終わる**のが狙い
    setDraft({
      name: s.name,
      qty: s.qty ?? "",
      kcal: s.kcal?.toString() ?? "",
      proteinG: s.proteinG?.toString() ?? "",
      fatG: s.fatG?.toString() ?? "",
      carbG: s.carbG?.toString() ?? "",
      // 履歴から選んだものは手入力と同じ確度で扱う
      source: "manual",
    });
    setPicking(false);
  }, []);

  function submit() {
    if (draft.name.trim() === "") return;
    setMessage(null);

    startTransition(async () => {
      const res = await createMeal({
        date,
        slot,
        name: draft.name.trim(),
        qty: draft.qty.trim() || null,
        kcal: num(draft.kcal),
        proteinG: num(draft.proteinG),
        fatG: num(draft.fatG),
        carbG: num(draft.carbG),
        source: draft.source,
      });

      if (!res.ok) {
        setMessage({ ok: false, text: res.message });

        return;
      }
      setMeals((prev) => [...prev, res.meal]);
      setDraft(EMPTY_DRAFT);
    });
  }

  function remove(id: string) {
    startTransition(async () => {
      const res = await deleteMeal(id);
      if (!res.ok) {
        setMessage({ ok: false, text: res.message ?? "削除できなかった" });

        return;
      }
      setMeals((prev) => prev.filter((m) => m.id !== id));
    });
  }

  function applySet(id: string) {
    setMessage(null);
    startTransition(async () => {
      const res = await applyMealSet(id, date, slot);
      setMessage(
        res.ok
          ? { ok: true, text: `${res.count} 件を記録した。画面を更新すると出る` }
          : { ok: false, text: res.message },
      );
    });
  }

  function saveAsSet() {
    const target = meals.filter((m) => m.slot === slot);
    if (target.length === 0) {
      setMessage({ ok: false, text: `${slot}の記録が無いのでセットにできない` });

      return;
    }

    const name = window.prompt("セットの名前", `${slot}セット`);
    if (name === null || name.trim() === "") return;

    setMessage(null);
    startTransition(async () => {
      const res = await createMealSetFrom(name.trim(), target, slot);
      setMessage(
        res.ok
          ? { ok: true, text: `「${name.trim()}」を保存した` }
          : { ok: false, text: res.message ?? "保存できなかった" },
      );
    });
  }

  function copyYesterday() {
    setMessage(null);
    startTransition(async () => {
      const res = await copyMeals(yesterday, date);
      setMessage(
        res.ok
          ? { ok: true, text: `前日から ${res.count} 件を写した。画面を更新すると出る` }
          : { ok: false, text: res.message },
      );
    });
  }

  return (
    <div className="px-4 pb-12 lg:grid lg:grid-cols-[28rem_minmax(0,1fr)] lg:items-start lg:gap-8 lg:px-6">
      <div className="flex flex-col gap-4">
        <div className="grid grid-cols-4 gap-2">
          <Total label="kcal" value={totals.kcal} target={targets?.target?.kcal} />
          <Total label="P" value={totals.proteinG} unit="g" target={targets?.target?.proteinG} />
          <Total label="F" value={totals.fatG} unit="g" target={targets?.target?.fatG} />
          <Total label="C" value={totals.carbG} unit="g" target={targets?.target?.carbG} />
        </div>
        {targetsUnavailable && (
          <p className="rounded-xl border border-warn/40 bg-warn/10 px-3 py-2 text-xs text-warn">
            目標を出せない: {targetsUnavailable}
            <Link href="/settings" className="ml-1 underline">
              設定へ
            </Link>
          </p>
        )}
        {targets?.target == null && targets?.note && (
          <p className="rounded-xl border border-line bg-surface px-3 py-2 text-xs text-muted">
            {targets.note}
          </p>
        )}
        {targets?.intakeFloorHit && (
          <p className="rounded-xl border border-warn/40 bg-warn/10 px-3 py-2 text-xs text-warn">
            摂取が下限に達している。これ以上削らず、歩数で赤字を作る
          </p>
        )}
        {unknown > 0 && (
          <p className="rounded-xl border border-warn/40 bg-warn/10 px-3 py-2 text-xs text-warn">
            PFC が未入力の記録が {unknown} 件ある。合計はその分少なく出ている
          </p>
        )}

        <section className="rounded-2xl border border-line bg-surface p-4">
          <div className="mb-3 flex gap-2">
            {SLOTS.map((s) => (
              <button
                key={s}
                type="button"
                aria-pressed={slot === s}
                onClick={() => setSlot(s)}
                className={`pressable flex-1 rounded-full border py-2 text-sm ${
                  slot === s
                    ? "border-accent bg-accent font-semibold text-accent-ink"
                    : "border-line bg-surface-2 text-muted"
                }`}
              >
                {s}
              </button>
            ))}
          </div>

          {mealSets.length > 0 && (
            <div className="mb-3 flex gap-2 overflow-x-auto">
              {mealSets.map((ms) => (
                <button
                  key={ms.id}
                  type="button"
                  onClick={() => applySet(ms.id)}
                  disabled={pending}
                  className="pressable shrink-0 rounded-full border border-line bg-surface-2 px-3.5 py-2 text-sm disabled:opacity-40"
                >
                  {ms.name}
                  <span className="tnum ml-1.5 text-xs text-muted">{ms.items.length}</span>
                </button>
              ))}
            </div>
          )}

          <button
            type="button"
            onClick={() => setPicking(true)}
            className="pressable mb-3 flex w-full items-center justify-between gap-3 rounded-xl border border-line bg-surface-2 px-3 py-3 text-left"
          >
            <span className="text-sm">過去の記録から選ぶ</span>
            <span className="shrink-0 text-muted" aria-hidden>
              ›
            </span>
          </button>

          {draft.source === "ai_estimated" && (
            /*
             * **少し直しても推定のままにする。** 元が推定なら確度は推定のもの。
             * 全部打ち直したときだけ手入力に落とせるようにボタンを置く
             */
            <div className="mb-2 flex items-center gap-2 rounded-lg bg-surface-2 px-2 py-1.5">
              <span className="flex-1 text-[11px] text-muted">
                写真からの推定値として記録される
              </span>
              <button
                type="button"
                onClick={() => setDraft({ ...draft, source: "manual" })}
                className="pressable shrink-0 rounded-lg border border-line px-2 py-1 text-[11px] text-muted"
              >
                手入力にする
              </button>
            </div>
          )}

          <Field
            label="食べたもの"
            value={draft.name}
            onChange={(v) => setDraft({ ...draft, name: v })}
          />
          <Field
            label="量"
            value={draft.qty}
            placeholder="1個 / 200g"
            onChange={(v) => setDraft({ ...draft, qty: v })}
          />
          <div className="mt-2 grid grid-cols-4 gap-2">
            <Num
              label="kcal"
              value={draft.kcal}
              onChange={(v) => setDraft({ ...draft, kcal: v })}
            />
            <Num
              label="P"
              value={draft.proteinG}
              onChange={(v) => setDraft({ ...draft, proteinG: v })}
            />
            <Num label="F" value={draft.fatG} onChange={(v) => setDraft({ ...draft, fatG: v })} />
            <Num label="C" value={draft.carbG} onChange={(v) => setDraft({ ...draft, carbG: v })} />
          </div>
        </section>

        <EstimatePanel estimateMeal={estimateMeal} onEstimated={setDraft} />

        <button
          type="button"
          onClick={submit}
          disabled={pending || draft.name.trim() === ""}
          className="pressable h-14 w-full rounded-2xl bg-accent text-base font-bold text-accent-ink disabled:opacity-40"
        >
          {pending ? "記録中…" : `${slot}に記録`}
        </button>

        <button
          type="button"
          onClick={saveAsSet}
          disabled={pending}
          className="pressable h-11 w-full rounded-xl border border-line text-sm text-muted disabled:opacity-40"
        >
          今の{slot}をセットとして保存
        </button>

        <button
          type="button"
          onClick={copyYesterday}
          disabled={pending}
          className="pressable h-11 w-full rounded-xl border border-line text-sm text-muted disabled:opacity-40"
        >
          前日（{yesterday}）の食事を写す
        </button>

        {message && (
          <p
            role="status"
            className={`rounded-xl border px-3 py-2 text-sm ${
              message.ok
                ? "border-accent/40 bg-accent/10 text-accent"
                : "border-danger/40 bg-danger/10 text-danger"
            }`}
          >
            {message.text}
          </p>
        )}
      </div>

      <div className="mt-4 flex flex-col gap-3 lg:mt-0">
        {groups.length === 0 && (
          <p className="rounded-2xl border border-line bg-surface p-8 text-center text-sm text-muted">
            今日はまだ記録が無い
          </p>
        )}
        {groups.map((g) => (
          <section
            key={g.slot ?? "その他"}
            className="rounded-2xl border border-line bg-surface p-4"
          >
            <div className="mb-2 flex items-baseline justify-between">
              <h2 className="text-sm font-medium">{g.slot ?? "区分なし"}</h2>
              <span className="tnum text-xs text-muted">{sumMeals(g.meals).kcal} kcal</span>
            </div>
            <ul className="flex flex-col gap-1.5">
              {g.meals.map((m) => (
                <li key={m.id} className="flex items-baseline justify-between gap-3 text-sm">
                  <span className="min-w-0 truncate">
                    {m.name}
                    {m.qty && <span className="ml-1.5 text-xs text-muted">{m.qty}</span>}
                  </span>
                  <span className="flex shrink-0 items-baseline gap-2">
                    <span className="tnum text-xs text-muted">
                      {m.kcal != null ? `${m.kcal} kcal` : "—"}
                    </span>
                    <button
                      type="button"
                      onClick={() => remove(m.id)}
                      disabled={pending}
                      aria-label={`${m.name}を削除`}
                      className="pressable rounded-lg px-1.5 text-muted disabled:opacity-40"
                    >
                      ×
                    </button>
                  </span>
                </li>
              ))}
            </ul>
          </section>
        ))}
      </div>

      {picking && (
        <MealPicker
          loadSuggestions={loadSuggestions}
          onPick={pick}
          onClose={() => setPicking(false)}
        />
      )}
    </div>
  );
}

function num(v: string): number | null {
  const t = v.normalize("NFKC").trim();
  if (t === "" || !/^\d*\.?\d*$/.test(t)) return null;
  const n = Number(t);

  return Number.isFinite(n) ? n : null;
}

/**
 * 実績と、目標があれば残量を出す（要件 N-05）。
 *
 * **残量を主役にする。** 知りたいのは「今日あと何を食べられるか」で、
 * 合計値そのものは途中経過にすぎない。
 */
function Total({
  label,
  value,
  unit,
  target,
}: {
  label: string;
  value: number;
  unit?: string;
  target?: number;
}) {
  const remaining = target == null ? null : Math.round((target - value) * 10) / 10;

  return (
    <div className="rounded-2xl border border-line bg-surface px-2 py-3 text-center">
      <div className="text-[11px] text-muted">{label}</div>
      <div
        className={`tnum text-xl font-semibold leading-tight ${
          remaining != null && remaining < 0 ? "text-warn" : ""
        }`}
      >
        {remaining ?? Math.round(value * 10) / 10}
        {unit && <span className="ml-0.5 text-xs font-normal text-muted">{unit}</span>}
      </div>
      <div className="tnum text-[10px] text-muted">
        {target == null ? "実績" : `${Math.round(value)} / ${Math.round(target)}${unit ?? ""}`}
      </div>
    </div>
  );
}

function Field({
  label,
  value,
  placeholder,
  onChange,
}: {
  label: string;
  value: string;
  placeholder?: string;
  onChange: (v: string) => void;
}) {
  return (
    <label className="mt-2 block">
      <span className="mb-1 block text-xs text-muted">{label}</span>
      <input
        type="text"
        value={value}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
        className="h-11 w-full rounded-xl border border-line bg-surface-2 px-3 text-base outline-none placeholder:text-muted/50 focus:border-accent"
      />
    </label>
  );
}

function Num({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <label className="block">
      <span className="mb-1 block text-center text-[11px] text-muted">{label}</span>
      <input
        type="text"
        inputMode="decimal"
        aria-label={label}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="tnum h-11 w-full rounded-xl border border-line bg-surface-2 px-2 text-center text-base outline-none focus:border-accent"
      />
    </label>
  );
}
