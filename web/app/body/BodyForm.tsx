"use client";

import { useState, useTransition } from "react";

import type {
  BodyMeasurement,
  BodyMeasurementInput,
  DailyMetrics,
  DailyMetricsInput,
} from "@/lib/api/client";

import { NumberField } from "./NumberField";
import type { SaveResult } from "./actions";

type Props = {
  date: string;
  /** 今日の日次記録。まだ無ければ null */
  today: DailyMetrics | null;
  /** 今日すでに記録した周囲長。無ければ null */
  todayMeasurement: BodyMeasurement | null;
  /** 今日より前の直近の周囲長。差分の表示に使う（要件 B-03） */
  previousMeasurement: BodyMeasurement | null;
  /** 今日より前の直近の体重・体脂肪率。差分の表示に使う（要件 B-03） */
  lastWeightKg: number | null;
  lastBodyfatPct: number | null;
  saveDaily: (input: DailyMetricsInput) => Promise<SaveResult>;
  saveMeasurement: (input: BodyMeasurementInput) => Promise<SaveResult>;
};

/** 周囲長の項目。順序は data/sample/measures.csv と同じにする */
const PARTS = [
  { key: "neckCm", label: "首", max: 100 },
  { key: "shoulderCm", label: "肩", max: 200 },
  { key: "chestCm", label: "胸", max: 200 },
  { key: "waistNavelCm", label: "ウエスト（へそ）", max: 200 },
  { key: "hipCm", label: "臀部", max: 200 },
  { key: "armRCm", label: "上腕（右）", max: 100 },
  { key: "thighRCm", label: "大腿（右）", max: 150 },
  { key: "calfRCm", label: "下腿（右）", max: 100 },
] as const;

type PartKey = (typeof PARTS)[number]["key"];

export function BodyForm({
  date,
  today,
  todayMeasurement,
  previousMeasurement,
  lastWeightKg,
  lastBodyfatPct,
  saveDaily,
  saveMeasurement,
}: Props) {
  const [weight, setWeight] = useState<number | null>(today?.weightKg ?? null);
  const [bodyfat, setBodyfat] = useState<number | null>(today?.bodyfatPct ?? null);
  const [fatigue, setFatigue] = useState<number | null>(today?.fatigue ?? null);
  const [parts, setParts] = useState<Record<PartKey, number | null>>(
    () =>
      Object.fromEntries(PARTS.map((p) => [p.key, todayMeasurement?.[p.key] ?? null])) as Record<
        PartKey,
        number | null
      >,
  );
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [pending, startTransition] = useTransition();

  const touchedParts = PARTS.filter((p) => parts[p.key] !== null);

  function submitDaily() {
    setMessage(null);
    startTransition(async () => {
      const res = await saveDaily({ date, weightKg: weight, bodyfatPct: bodyfat, fatigue });
      setMessage(
        res.ok ? { ok: true, text: "体組成を保存した" } : { ok: false, text: res.message },
      );
    });
  }

  function submitMeasurement() {
    setMessage(null);
    startTransition(async () => {
      // 触っていない項目は送らない。API 側で null は「変更しない」になる
      const body: BodyMeasurementInput = { date };
      for (const p of touchedParts) body[p.key] = parts[p.key];

      const res = await saveMeasurement(body);
      setMessage(
        res.ok ? { ok: true, text: "周囲長を保存した" } : { ok: false, text: res.message },
      );
    });
  }

  return (
    <div className="px-4 pb-12 lg:grid lg:grid-cols-2 lg:items-start lg:gap-8 lg:px-6">
      <div className="flex flex-col gap-4">
        <section className="rounded-2xl border border-line bg-surface p-4">
          <h2 className="mb-1 text-sm font-medium">体組成</h2>
          <NumberField
            label="体重"
            unit="kg"
            value={weight}
            step={0.1}
            max={300}
            previous={lastWeightKg}
            disabled={pending}
            onChange={setWeight}
          />
          <NumberField
            label="体脂肪率"
            unit="%"
            value={bodyfat}
            step={0.1}
            max={70}
            previous={lastBodyfatPct}
            disabled={pending}
            onChange={setBodyfat}
          />
        </section>

        <FatiguePicker value={fatigue} disabled={pending} onChange={setFatigue} />

        <button
          type="button"
          onClick={submitDaily}
          disabled={pending}
          className="pressable h-14 w-full rounded-2xl bg-accent text-base font-bold text-accent-ink disabled:opacity-50"
        >
          {pending ? "保存中…" : "体組成を保存"}
        </button>
      </div>

      <div className="mt-4 flex flex-col gap-4 lg:mt-0">
        <section className="rounded-2xl border border-line bg-surface p-4">
          <div className="mb-1 flex items-baseline justify-between">
            <h2 className="text-sm font-medium">周囲長</h2>
            <span className="text-[11px] text-muted">
              {previousMeasurement ? `前回 ${previousMeasurement.date}` : "起床直後・食事前に測る"}
            </span>
          </div>
          {PARTS.map((p) => (
            <NumberField
              key={p.key}
              label={p.label}
              unit="cm"
              value={parts[p.key]}
              step={0.5}
              max={p.max}
              previous={previousMeasurement?.[p.key] ?? null}
              disabled={pending}
              onChange={(v) => setParts((prev) => ({ ...prev, [p.key]: v }))}
            />
          ))}
        </section>

        <button
          type="button"
          onClick={submitMeasurement}
          disabled={pending || touchedParts.length === 0}
          className="pressable h-14 w-full rounded-2xl bg-accent text-base font-bold text-accent-ink disabled:opacity-40"
        >
          {pending ? "保存中…" : `周囲長を保存（${touchedParts.length}/8）`}
        </button>
      </div>

      {message && (
        <p
          role="status"
          className={`mt-4 rounded-xl border px-3 py-2 text-sm lg:col-span-2 ${
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

/**
 * 疲労度（要件 B-06）。
 *
 * **1タップで入る形にする。** 毎日聞かれるものなので、数字を選ぶ以上の
 * 操作を挟むと入力されなくなる。
 */
function FatiguePicker({
  value,
  disabled,
  onChange,
}: {
  value: number | null;
  disabled?: boolean;
  onChange: (v: number | null) => void;
}) {
  return (
    <section className="rounded-2xl border border-line bg-surface p-4">
      <div className="mb-2 flex items-baseline justify-between">
        <h2 className="text-sm font-medium">疲労度</h2>
        <span className="text-[11px] text-muted">1 = 元気 / 5 = 動けない</span>
      </div>
      <div className="grid grid-cols-5 gap-2">
        {[1, 2, 3, 4, 5].map((n) => (
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
      </div>
    </section>
  );
}
