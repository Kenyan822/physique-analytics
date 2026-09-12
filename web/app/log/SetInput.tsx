"use client";

import { useState } from "react";

/**
 * 重量・レップ・RIR の入力（要件 T-01 / T-03）。
 *
 * **キーボードを開かずに済むことが最優先。** ジムでは片手・手袋・汗で
 * 数値入力は現実的でない。+/- ボタンで刻む（重量 2.5kg / レップ 1）。
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

/** 2.5 刻みで丸める。0.1 の誤差が出ると表示が 82.50000000000001 になる */
function roundStep(v: number, step: number): number {
  return Math.round(v / step) * step;
}

export function clampWeight(v: number): number {
  return Math.min(500, Math.max(0, roundStep(v, WEIGHT_STEP)));
}

export function clampReps(v: number): number {
  return Math.min(100, Math.max(0, Math.round(v)));
}

export function clampRir(v: number): number {
  return Math.min(MAX_RIR, Math.max(0, Math.round(v)));
}

export function SetInput({ value, onChange, disabled }: Props) {
  return (
    <div className="flex flex-col gap-3">
      <Stepper
        label="重量"
        unit="kg"
        value={value.weightKg}
        step={WEIGHT_STEP}
        disabled={disabled}
        onChange={(v) => onChange({ ...value, weightKg: clampWeight(v) })}
      />
      <Stepper
        label="レップ"
        unit="回"
        value={value.reps}
        step={REP_STEP}
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

function Stepper({
  label,
  unit,
  value,
  step,
  disabled,
  onChange,
}: {
  label: string;
  unit: string;
  value: number;
  step: number;
  disabled?: boolean;
  onChange: (v: number) => void;
}) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="w-16 text-sm text-gray-600 dark:text-gray-400">{label}</span>
      <div className="flex flex-1 items-center gap-2">
        <button
          type="button"
          aria-label={`${label}を減らす`}
          disabled={disabled}
          onClick={() => onChange(value - step)}
          className="h-12 w-12 shrink-0 rounded-lg border border-gray-300 text-xl font-medium disabled:opacity-40 dark:border-gray-700"
        >
          −
        </button>
        <output className="flex-1 text-center text-2xl font-semibold tabular-nums">
          {value}
          <span className="ml-1 text-base font-normal text-gray-500">{unit}</span>
        </output>
        <button
          type="button"
          aria-label={`${label}を増やす`}
          disabled={disabled}
          onClick={() => onChange(value + step)}
          className="h-12 w-12 shrink-0 rounded-lg border border-gray-300 text-xl font-medium disabled:opacity-40 dark:border-gray-700"
        >
          ＋
        </button>
      </div>
    </div>
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
  const [showAll, setShowAll] = useState(false);
  const choices = showAll ? [...Array(MAX_RIR + 1).keys()] : [0, 1, 2, 3, 4];

  return (
    <div className="flex items-start justify-between gap-2">
      <span className="w-16 pt-3 text-sm text-gray-600 dark:text-gray-400">RIR</span>
      <div className="flex flex-1 flex-wrap gap-2">
        {choices.map((n) => (
          <button
            key={n}
            type="button"
            aria-pressed={value === n}
            disabled={disabled}
            onClick={() => onChange(value === n ? null : n)}
            className={`h-12 w-12 rounded-lg border text-lg tabular-nums disabled:opacity-40 ${
              value === n
                ? "border-transparent bg-gray-900 text-white dark:bg-white dark:text-gray-900"
                : "border-gray-300 dark:border-gray-700"
            }`}
          >
            {n}
          </button>
        ))}
        {!showAll && (
          <button
            type="button"
            disabled={disabled}
            onClick={() => setShowAll(true)}
            className="h-12 rounded-lg border border-gray-300 px-3 text-sm disabled:opacity-40 dark:border-gray-700"
          >
            5+
          </button>
        )}
      </div>
    </div>
  );
}
