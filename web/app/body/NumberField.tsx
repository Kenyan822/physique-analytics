"use client";

import { useState } from "react";

import { formatDiff, parseInput, round2 } from "@/lib/number";

type Props = {
  label: string;
  unit: string;
  value: number | null;
  /** +/- ボタンの刻み */
  step: number;
  max: number;
  /** 前回値。差分の表示とプレースホルダに使う（要件 B-03） */
  previous?: number | null;
  disabled?: boolean;
  onChange: (v: number | null) => void;
};

/**
 * 数値の入力欄。
 *
 * 記録画面（SetInput）と違って**未入力を許す**。体脂肪率や周囲長は
 * 測れた日だけ入れるもので、0 と「測っていない」は別の意味になる。
 */
export function NumberField({
  label,
  unit,
  value,
  step,
  max,
  previous,
  disabled,
  onChange,
}: Props) {
  const [draft, setDraft] = useState<string | null>(null);
  const shown = draft ?? (value === null ? "" : String(value));
  const diff = formatDiff(value, previous);

  function commit() {
    if (draft === null) return;
    const text = draft.trim();
    // 空にしたら「未入力に戻す」。0 を入れたことにしない
    onChange(text === "" ? null : clamp(parseInput(text, value ?? 0), max));
    setDraft(null);
  }

  function bump(by: number) {
    onChange(clamp((value ?? previous ?? 0) + by, max));
  }

  return (
    <div className="flex items-center gap-2 border-b border-line/60 py-2.5 last:border-0">
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm">{label}</div>
        <div className="text-[11px] text-muted">
          {previous == null ? "前回なし" : `前回 ${previous}${unit}`}
          {diff && <span className="ml-1.5 font-medium text-accent">{diff}</span>}
        </div>
      </div>

      <StepButton label={`${label}を減らす`} disabled={disabled} onClick={() => bump(-step)}>
        −
      </StepButton>

      <div className="flex w-[5.5rem] items-baseline justify-end gap-1">
        <input
          type="text"
          inputMode="decimal"
          aria-label={label}
          disabled={disabled}
          value={shown}
          placeholder={previous == null ? "—" : String(previous)}
          onFocus={(e) => {
            setDraft(shown);
            e.currentTarget.select();
          }}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => {
            if (e.key === "Enter") e.currentTarget.blur();
          }}
          className="tnum w-full rounded-lg border border-transparent bg-transparent py-1 text-right text-2xl font-semibold outline-none placeholder:text-muted/50 focus:border-accent disabled:opacity-40"
        />
        <span className="shrink-0 text-xs text-muted">{unit}</span>
      </div>

      <StepButton label={`${label}を増やす`} disabled={disabled} onClick={() => bump(step)}>
        ＋
      </StepButton>
    </div>
  );
}

function clamp(v: number, max: number): number {
  return Math.min(max, Math.max(0, round2(v)));
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
      className="pressable flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-line bg-surface-2 text-lg disabled:opacity-40"
    >
      {children}
    </button>
  );
}
