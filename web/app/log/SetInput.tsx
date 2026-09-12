"use client";

import { useState } from "react";

/**
 * 重量・レップ・RIR の入力（要件 T-01 / T-03）。
 *
 * **+/- ボタンを主役にする。** ジムでは片手・手袋・汗で、キーボードを
 * 出しての数値入力は現実的でない。ただし 60 → 82.5 のような飛んだ値は
 * ボタンでは遠すぎるので、数字をタップすれば直接打てるようにしてある。
 *
 * 重量を一番上に置いているのは、セット間で一番よく変える値だから。
 */
export const WEIGHT_STEP = 2.5;
export const REP_STEP = 1;
export const MAX_RIR = 10;

export type SetValue = {
  weightKg: number;
  reps: number;
  rir: number | null;
};

type Props = {
  value: SetValue;
  onChange: (v: SetValue) => void;
  disabled?: boolean;
};

/**
 * 0.1 + 0.2 が 0.30000000000000004 になるのを消す。
 * 刻みには丸めない。ダンベルもマシンも 2.5kg 刻みとは限らない。
 *
 * `v * 100` ではなく文字列で指数をずらしているのは、二進で丸めると
 * 1.005 が 1.00 になるため（1.005 * 100 は 100.49999… になる）。
 * 指数表記になる極端な値は桁シフトが NaN になるので、丸めずに返す
 */
function round2(v: number): number {
  const shifted = Number(`${v}e2`);

  return Number.isFinite(shifted) ? Number(`${Math.round(shifted)}e-2`) : v;
}

export function clampWeight(v: number): number {
  return Math.min(500, Math.max(0, round2(v)));
}

export function clampReps(v: number): number {
  return Math.min(100, Math.max(0, Math.round(v)));
}

export function clampRir(v: number): number {
  return Math.min(MAX_RIR, Math.max(0, Math.round(v)));
}

/**
 * 入力欄の文字列を数値にする。読めなければ `fallback`（＝直前の値）に戻す。
 *
 * NFKC で正規化しているのは、日本語入力のまま打つと全角数字になるため。
 * 弾くと「打てない」に見えるので、読めるものは読む。
 */
export function parseInput(text: string, fallback: number): number {
  const normalized = text.normalize("NFKC").trim();
  if (!/^\d*\.?\d*$/.test(normalized) || normalized === "" || normalized === ".") {
    return fallback;
  }

  const n = Number(normalized);

  return Number.isFinite(n) ? n : fallback;
}

export function SetInput({ value, onChange, disabled }: Props) {
  return (
    <div className="flex flex-col gap-3">
      <Hero
        label="重量"
        unit="kg"
        value={value.weightKg}
        step={WEIGHT_STEP}
        inputMode="decimal"
        disabled={disabled}
        onChange={(v) => onChange({ ...value, weightKg: clampWeight(v) })}
      />
      <Hero
        label="レップ"
        unit="回"
        value={value.reps}
        step={REP_STEP}
        inputMode="numeric"
        disabled={disabled}
        onChange={(v) => onChange({ ...value, reps: clampReps(v) })}
      />
      <RirPicker
        value={value.rir}
        disabled={disabled}
        onChange={(rir) => onChange({ ...value, rir })}
      />
    </div>
  );
}

/**
 * 大きい数字と両脇のボタン。数字自体が入力欄になっている。
 *
 * ボタンは 64px 角。Apple の推奨（44pt）より大きくしているのは、
 * 息が上がった状態で片手で押すため。
 */
function Hero({
  label,
  unit,
  value,
  step,
  inputMode,
  disabled,
  onChange,
}: {
  label: string;
  unit: string;
  value: number;
  step: number;
  inputMode: "decimal" | "numeric";
  disabled?: boolean;
  onChange: (v: number) => void;
}) {
  /**
   * 入力中だけ文字列を持つ。数値に直しながら表示すると
   * 「6」と打った瞬間に丸められて、続きの「2.5」が打てない
   */
  const [draft, setDraft] = useState<string | null>(null);
  const shown = draft ?? String(value);

  function commit() {
    if (draft !== null) onChange(parseInput(draft, value));
    setDraft(null);
  }

  return (
    <div className="rounded-2xl border border-line bg-surface p-4">
      <div className="mb-2 text-xs text-muted">{label}</div>
      <div className="flex items-center gap-3">
        <StepButton
          label={`${label}を減らす`}
          disabled={disabled}
          onClick={() => onChange(value - step)}
        >
          −
        </StepButton>

        <div className="flex flex-1 items-baseline justify-center gap-1.5">
          <input
            type="text"
            inputMode={inputMode}
            aria-label={label}
            disabled={disabled}
            value={shown}
            /* 桁数に合わせて伸ばす。中央寄せの固定幅だと単位が離れて見える */
            style={{ width: `${Math.max(2, shown.length)}ch` }}
            onFocus={(e) => {
              setDraft(String(value));
              e.currentTarget.select();
            }}
            onChange={(e) => setDraft(e.target.value)}
            onBlur={commit}
            onKeyDown={(e) => {
              if (e.key === "Enter") e.currentTarget.blur();
            }}
            className="tnum rounded-lg border border-transparent bg-transparent py-1 text-center text-5xl font-semibold leading-none outline-none focus:border-accent disabled:opacity-40"
          />
          <span className="text-xl font-normal text-muted">{unit}</span>
        </div>

        <StepButton
          label={`${label}を増やす`}
          disabled={disabled}
          onClick={() => onChange(value + step)}
        >
          ＋
        </StepButton>
      </div>
    </div>
  );
}

function StepButton({
  label,
  disabled,
  onClick,
  children,
}: {
  label: string;
  disabled?: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      disabled={disabled}
      onClick={onClick}
      className="pressable flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl border border-line bg-surface-2 text-2xl font-medium disabled:opacity-40"
    >
      {children}
    </button>
  );
}

/**
 * RIR は 0〜4 をボタンで選ぶ。
 *
 * **これが無いと推定1RMが計算できず進捗が測れない**（openapi.yaml）。
 * 実際に使うのはほぼ 0〜4 なので、その範囲だけ1タップにする。
 */
function RirPicker({
  value,
  disabled,
  onChange,
}: {
  value: number | null;
  disabled?: boolean;
  onChange: (v: number | null) => void;
}) {
  return (
    <div className="rounded-2xl border border-line bg-surface p-4">
      <div className="mb-2 flex items-baseline justify-between">
        <span className="text-xs text-muted">RIR</span>
        <span className="text-[11px] text-muted">あと何回できたか</span>
      </div>
      <div className="grid grid-cols-6 gap-2">
        {[0, 1, 2, 3, 4].map((n) => (
          <button
            key={n}
            type="button"
            aria-pressed={value === n}
            disabled={disabled}
            onClick={() => onChange(value === n ? null : n)}
            className={`pressable tnum h-14 rounded-xl border text-lg disabled:opacity-40 ${
              value === n
                ? "border-accent bg-accent font-semibold text-accent-ink"
                : "border-line bg-surface-2"
            }`}
          >
            {n}
          </button>
        ))}
        <select
          aria-label="RIR（5以上）"
          disabled={disabled}
          value={value !== null && value >= 5 ? String(value) : ""}
          onChange={(e) => onChange(e.target.value === "" ? null : Number(e.target.value))}
          className={`pressable h-14 rounded-xl border bg-surface-2 text-center text-sm disabled:opacity-40 ${
            value !== null && value >= 5 ? "border-accent text-accent" : "border-line text-muted"
          }`}
        >
          <option value="">5+</option>
          {[5, 6, 7, 8, 9, 10].map((n) => (
            <option key={n} value={n}>
              {n}
            </option>
          ))}
        </select>
      </div>
    </div>
  );
}
