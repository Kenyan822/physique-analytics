"use client";

import { useState, useTransition } from "react";

import type { Plan, PlanInput, PlanPhase, VolumeRange } from "@/lib/api/client";

import type { SavePlanResult } from "./actions";
import { findInvalidPeriod, findOverlap, nextPhaseDefaults, sortPhases } from "./phases";
import { isMonth, toPlanInput } from "./planInput";

type Props = {
  plan: Plan;
  today: string;
  savePlan: (input: PlanInput) => Promise<SavePlanResult>;
};

export function SettingsForm({ plan, today, savePlan }: Props) {
  const [heightCm, setHeightCm] = useState(plan.heightCm?.toString() ?? "");
  const [baselineWeightKg, setBaselineWeightKg] = useState(plan.baselineWeightKg?.toString() ?? "");
  const [baselineBodyfatPct, setBaselineBodyfatPct] = useState(
    plan.baselineBodyfatPct?.toString() ?? "",
  );
  const [baselineMonth, setBaselineMonth] = useState(plan.baselineMonth ?? "");
  const [phases, setPhases] = useState<PlanPhase[]>(sortPhases(plan.phases));
  const [nutrition, setNutrition] = useState(plan.nutrition);
  const [ranges, setRanges] = useState<VolumeRange[]>(plan.volumeRanges);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [pending, startTransition] = useTransition();

  // **保存する前に画面で気づけるようにする。** サーバでも同じ検証をしているが、
  // 往復してから怒られるより、その場で分かる方が速い
  const overlap = findOverlap(phases);
  const invalid = findInvalidPeriod(phases);
  const badMonth = baselineMonth !== "" && !isMonth(baselineMonth);
  const blocked = overlap !== null || invalid !== null || badMonth;

  function submit() {
    setMessage(null);
    startTransition(async () => {
      const res = await savePlan(
        toPlanInput(plan, {
          heightCm,
          baselineWeightKg,
          baselineBodyfatPct,
          baselineMonth,
          phases: sortPhases(phases),
          nutrition,
          volumeRanges: ranges,
        }),
      );
      setMessage(res.ok ? { ok: true, text: "保存した" } : { ok: false, text: res.message });
      if (res.ok) setPhases(sortPhases(res.plan.phases));
    });
  }

  return (
    <div className="flex flex-col gap-4 px-4 pb-28 lg:px-0">
      <section className="rounded-2xl border border-line bg-surface p-4">
        <h2 className="mb-3 text-sm font-medium">身体</h2>
        <label className="block">
          <span className="mb-1 block text-xs text-muted">
            身長（正規化FFMI と海軍式推定に使う）
          </span>
          <input
            type="text"
            inputMode="decimal"
            aria-label="身長"
            value={heightCm}
            onChange={(e) => setHeightCm(e.target.value)}
            className="tnum h-11 w-32 rounded-xl border border-line bg-surface-2 px-3 text-base outline-none focus:border-accent"
          />
          <span className="ml-2 text-sm text-muted">cm</span>
        </label>
      </section>

      {/*
       * 月次目標の起点（要件 P-02 / P-03）。**ここを画面に出しておかないと
       * 保存のたびに消える。** PUT /v1/plan はまるごと置き換わるため
       */}
      <section className="rounded-2xl border border-line bg-surface p-4">
        <div className="mb-3 flex items-baseline justify-between gap-2">
          <h2 className="text-sm font-medium">月次目標の起点</h2>
          <span className="text-[11px] text-muted">3年計画の出発点</span>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <TextField
            label="起点体重"
            unit="kg"
            value={baselineWeightKg}
            onChange={setBaselineWeightKg}
          />
          <TextField
            label="起点体脂肪率"
            unit="%"
            value={baselineBodyfatPct}
            onChange={setBaselineBodyfatPct}
          />
          <TextField
            label="起点月"
            unit="起点月"
            width="w-24"
            placeholder="2026-09"
            invalid={badMonth}
            value={baselineMonth}
            onChange={setBaselineMonth}
          />
        </div>
        {badMonth && <p className="mt-2 text-xs text-danger">起点月は YYYY-MM で入れる</p>}
      </section>

      <section className="rounded-2xl border border-line bg-surface p-4">
        <div className="mb-3 flex items-baseline justify-between">
          <h2 className="text-sm font-medium">フェーズ</h2>
          <span className="text-[11px] text-muted">期間ごとの目標ペース</span>
        </div>

        {phases.length === 0 && (
          <p className="mb-3 rounded-xl border border-warn/40 bg-warn/10 px-3 py-2 text-xs text-warn">
            フェーズが無いと、推奨摂取も停滞判定も出せない
          </p>
        )}

        <ul className="flex flex-col gap-2">
          {phases.map((p, i) => (
            <li key={i} className="rounded-xl border border-line bg-surface-2 p-3">
              <div className="flex items-center gap-2">
                <input
                  type="text"
                  aria-label={`フェーズ${i + 1}の名前`}
                  value={p.name}
                  placeholder="P1-A カット1.0%"
                  onChange={(e) => updatePhase(setPhases, i, { name: e.target.value })}
                  className="h-10 min-w-0 flex-1 rounded-lg border border-line bg-surface px-2 text-sm outline-none focus:border-accent"
                />
                <button
                  type="button"
                  aria-label={`フェーズ${i + 1}を削除`}
                  onClick={() => setPhases((prev) => prev.filter((_, j) => j !== i))}
                  className="pressable shrink-0 rounded-lg px-2 text-muted"
                >
                  ×
                </button>
              </div>
              <div className="mt-2 flex flex-wrap items-center gap-2 text-sm">
                <input
                  type="date"
                  aria-label={`フェーズ${i + 1}の開始日`}
                  value={p.startsOn}
                  onChange={(e) => updatePhase(setPhases, i, { startsOn: e.target.value })}
                  className="tnum h-10 rounded-lg border border-line bg-surface px-2 outline-none focus:border-accent"
                />
                <span className="text-muted">〜</span>
                <input
                  type="date"
                  aria-label={`フェーズ${i + 1}の終了日`}
                  value={p.endsOn}
                  onChange={(e) => updatePhase(setPhases, i, { endsOn: e.target.value })}
                  className="tnum h-10 rounded-lg border border-line bg-surface px-2 outline-none focus:border-accent"
                />
                <label className="ml-auto flex items-center gap-1.5">
                  <input
                    type="text"
                    inputMode="decimal"
                    aria-label={`フェーズ${i + 1}の目標ペース`}
                    value={p.goalKgPerWeek}
                    onChange={(e) =>
                      updatePhase(setPhases, i, { goalKgPerWeek: Number(e.target.value) || 0 })
                    }
                    className="tnum h-10 w-20 rounded-lg border border-line bg-surface px-2 text-right outline-none focus:border-accent"
                  />
                  <span className="text-xs text-muted">kg/週</span>
                </label>
              </div>
            </li>
          ))}
        </ul>

        <button
          type="button"
          onClick={() => setPhases((prev) => [...prev, nextPhaseDefaults(prev, today)])}
          className="pressable mt-3 h-10 w-full rounded-xl border border-line text-sm text-muted"
        >
          フェーズを追加
        </button>

        {invalid && (
          <p className="mt-2 rounded-xl border border-danger/40 bg-danger/10 px-3 py-2 text-xs text-danger">
            「{invalid.name || "名前なし"}」の終了日が開始日より前
          </p>
        )}
        {overlap && (
          <p className="mt-2 rounded-xl border border-danger/40 bg-danger/10 px-3 py-2 text-xs text-danger">
            期間が重なっている（{overlap[0].name || "名前なし"} と {overlap[1].name || "名前なし"}
            ）。 重なるとその日の目標ペースが決まらない
          </p>
        )}
      </section>

      <section className="rounded-2xl border border-line bg-surface p-4">
        <div className="mb-3 flex items-baseline justify-between">
          <h2 className="text-sm font-medium">栄養パラメータ</h2>
          <span className="text-[11px] text-muted">体重あたり。炭水化物は残余</span>
        </div>
        <MacroRow
          label="減量期"
          value={nutrition.cut}
          onChange={(v) => setNutrition({ ...nutrition, cut: v })}
        />
        <MacroRow
          label="深い減量期"
          value={nutrition.deepCut}
          onChange={(v) => setNutrition({ ...nutrition, deepCut: v })}
        />
        <MacroRow
          label="増量期"
          value={nutrition.bulk}
          onChange={(v) => setNutrition({ ...nutrition, bulk: v })}
        />
        <div className="mt-3 flex flex-wrap gap-4">
          <NumField
            label="深い減量に切り替える体脂肪率"
            unit="%"
            value={nutrition.deepCutBfThreshold}
            onChange={(v) => setNutrition({ ...nutrition, deepCutBfThreshold: v })}
          />
          <NumField
            label="炭水化物の下限"
            unit="g"
            value={nutrition.carbMinG}
            onChange={(v) => setNutrition({ ...nutrition, carbMinG: Math.round(v) })}
          />
        </div>
      </section>

      <section className="rounded-2xl border border-line bg-surface p-4">
        <div className="mb-3 flex items-baseline justify-between">
          <h2 className="text-sm font-medium">部位別 MEV / MRV</h2>
          <span className="text-[11px] text-muted">週あたりの有効セット数</span>
        </div>
        <ul className="grid gap-2 sm:grid-cols-2">
          {ranges.map((r, i) => (
            <li key={r.muscleGroup} className="flex items-center gap-2">
              <span className="min-w-0 flex-1 truncate text-sm">{r.muscleGroup}</span>
              <SetsInput
                label={`${r.muscleGroup}のMEV`}
                value={r.mev}
                onChange={(v) => updateRange(setRanges, i, { mev: v })}
              />
              <span className="text-xs text-muted">〜</span>
              <SetsInput
                label={`${r.muscleGroup}のMRV`}
                value={r.mrv}
                onChange={(v) => updateRange(setRanges, i, { mrv: v })}
              />
              {r.mev > r.mrv && (
                <span className="shrink-0 text-xs text-danger" title="MEV が MRV を超えている">
                  !
                </span>
              )}
            </li>
          ))}
        </ul>
      </section>

      <button
        type="button"
        onClick={submit}
        disabled={pending || blocked}
        className="pressable h-14 w-full rounded-2xl bg-accent text-base font-bold text-accent-ink disabled:opacity-40"
      >
        {pending ? "保存中…" : blocked ? "エラーを直すと保存できる" : "保存"}
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
  );
}

function updatePhase(
  set: React.Dispatch<React.SetStateAction<PlanPhase[]>>,
  index: number,
  patch: Partial<PlanPhase>,
) {
  set((prev) => prev.map((p, i) => (i === index ? { ...p, ...patch } : p)));
}

function updateRange(
  set: React.Dispatch<React.SetStateAction<VolumeRange[]>>,
  index: number,
  patch: Partial<Pick<VolumeRange, "mev" | "mrv">>,
) {
  set((prev) => prev.map((r, i) => (i === index ? { ...r, ...patch } : r)));
}

type Macro = { proteinGPerKg: number; fatGPerKg: number };

function MacroRow({
  label,
  value,
  onChange,
}: {
  label: string;
  value: Macro;
  onChange: (v: Macro) => void;
}) {
  return (
    <div className="flex items-center gap-3 border-b border-line/60 py-2 last:border-0">
      <span className="min-w-0 flex-1 truncate text-sm">{label}</span>
      <NumField
        label={`${label}のタンパク質`}
        unit="P g/kg"
        value={value.proteinGPerKg}
        onChange={(v) => onChange({ ...value, proteinGPerKg: v })}
      />
      <NumField
        label={`${label}の脂質`}
        unit="F g/kg"
        value={value.fatGPerKg}
        onChange={(v) => onChange({ ...value, fatGPerKg: v })}
      />
    </div>
  );
}

function NumField({
  label,
  unit,
  value,
  onChange,
}: {
  label: string;
  unit: string;
  value: number;
  onChange: (v: number) => void;
}) {
  return (
    <label className="flex shrink-0 items-center gap-1.5">
      <input
        type="text"
        inputMode="decimal"
        aria-label={label}
        value={value}
        onChange={(e) => onChange(Number(e.target.value.normalize("NFKC")) || 0)}
        className="tnum h-10 w-16 rounded-lg border border-line bg-surface-2 px-2 text-right outline-none focus:border-accent"
      />
      <span className="text-[11px] text-muted">{unit}</span>
    </label>
  );
}

function SetsInput({
  label,
  value,
  onChange,
}: {
  label: string;
  value: number;
  onChange: (v: number) => void;
}) {
  return (
    <input
      type="text"
      inputMode="numeric"
      aria-label={label}
      value={value}
      onChange={(e) => onChange(Math.round(Number(e.target.value.normalize("NFKC")) || 0))}
      className="tnum h-10 w-12 shrink-0 rounded-lg border border-line bg-surface-2 px-1 text-center outline-none focus:border-accent"
    />
  );
}

/**
 * 入力中の文字列をそのまま持つ数値欄。
 *
 * **数値に直しながら持たない。** 「20.」と打った時点で 20 に丸められ、
 * 続きの小数が打てなくなる。読み取りは保存時にまとめてやる（toPlanInput）。
 */
function TextField({
  label,
  unit,
  value,
  onChange,
  width = "w-20",
  placeholder,
  invalid,
}: {
  label: string;
  unit: string;
  value: string;
  onChange: (v: string) => void;
  width?: string;
  placeholder?: string;
  invalid?: boolean;
}) {
  return (
    <label className="flex shrink-0 items-center gap-1.5">
      <input
        type="text"
        inputMode="decimal"
        aria-label={label}
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={`tnum h-10 rounded-lg border bg-surface-2 px-2 text-right outline-none focus:border-accent ${width} ${
          invalid ? "border-danger" : "border-line"
        }`}
      />
      <span className="text-[11px] text-muted">{unit}</span>
    </label>
  );
}
