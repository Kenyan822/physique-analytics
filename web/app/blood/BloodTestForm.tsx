"use client";

import { useState, useTransition } from "react";

import type { BloodTest, BloodTestInput } from "@/lib/api/client";

import type { BloodTestResult } from "./actions";
import { emptyRow, toItems, type ItemRow } from "./rows";

type Props = {
  today: string;
  createBloodTest: (input: BloodTestInput) => Promise<BloodTestResult>;
  onCreated: (test: BloodTest) => void;
};

/** 最初から出しておく行数。足りなければ増やす */
const INITIAL_ROWS = 8;

/**
 * 検査票を写す（要件 B-08）。
 *
 * **項目を固定しない。** クリニックやパネルで項目が違うので、
 * 名前ごと打てる行を並べる。基準範囲も検査票に書いてある値を入れる。
 */
export function BloodTestForm({ today, createBloodTest, onCreated }: Props) {
  const [date, setDate] = useState(today);
  const [clinic, setClinic] = useState("");
  const [note, setNote] = useState("");
  const [rows, setRows] = useState<ItemRow[]>(() =>
    Array.from({ length: INITIAL_ROWS }, () => emptyRow()),
  );
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [pending, startTransition] = useTransition();

  const items = toItems(rows);

  function update(index: number, patch: Partial<ItemRow>) {
    setRows((prev) => prev.map((r, i) => (i === index ? { ...r, ...patch } : r)));
  }

  function save() {
    if (items.length === 0) return;
    setMessage(null);

    startTransition(async () => {
      const res = await createBloodTest({
        date,
        clinic: clinic.trim() === "" ? null : clinic.trim(),
        note: note.trim() === "" ? null : note.trim(),
        items,
      });
      if (!res.ok) {
        setMessage({ ok: false, text: res.message });

        return;
      }
      onCreated(res.test);
      setClinic("");
      setNote("");
      setRows(Array.from({ length: INITIAL_ROWS }, () => emptyRow()));
      setMessage({ ok: true, text: `${res.test.date} の検査を登録した` });
    });
  }

  return (
    <section className="rounded-2xl border border-line bg-surface p-4">
      <div className="mb-3 flex items-baseline justify-between gap-2">
        <h2 className="text-sm font-medium">検査票を写す</h2>
        <span className="tnum text-[11px] text-muted">{items.length} 項目</span>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <input
          type="date"
          aria-label="採血日"
          value={date}
          onChange={(e) => setDate(e.target.value)}
          className="tnum h-10 rounded-lg border border-line bg-surface-2 px-2 text-sm outline-none focus:border-accent"
        />
        <input
          type="text"
          aria-label="クリニック"
          value={clinic}
          placeholder="クリニック"
          onChange={(e) => setClinic(e.target.value)}
          className="h-10 min-w-0 flex-1 rounded-lg border border-line bg-surface-2 px-2 text-sm outline-none focus:border-accent"
        />
      </div>

      {/* 項目は横に5列。狭い画面では表ごと横に流す */}
      <div className="-mx-4 mt-3 overflow-x-auto px-4">
        <div className="min-w-[34rem]">
          <div className="flex gap-2 pb-1 text-[11px] text-muted">
            <span className="flex-1">項目</span>
            <span className="w-20 text-right">値</span>
            <span className="w-16">単位</span>
            <span className="w-32 text-right">基準範囲</span>
          </div>
          <ul className="flex flex-col gap-1.5">
            {rows.map((r, i) => (
              <li key={i} className="flex items-center gap-2">
                <Cell
                  label={`${i + 1}行目の項目名`}
                  value={r.name}
                  placeholder="ヘモグロビン"
                  onChange={(v) => update(i, { name: v })}
                  className="flex-1"
                />
                <Cell
                  label={`${i + 1}行目の値`}
                  value={r.value}
                  placeholder="15.2"
                  onChange={(v) => update(i, { value: v })}
                  className="tnum w-20 text-right"
                />
                <Cell
                  label={`${i + 1}行目の単位`}
                  value={r.unit}
                  placeholder="g/dL"
                  onChange={(v) => update(i, { unit: v })}
                  className="w-16"
                />
                <Cell
                  label={`${i + 1}行目の基準下限`}
                  value={r.refLow}
                  placeholder="下限"
                  onChange={(v) => update(i, { refLow: v })}
                  className="tnum w-15 text-right"
                />
                <Cell
                  label={`${i + 1}行目の基準上限`}
                  value={r.refHigh}
                  placeholder="上限"
                  onChange={(v) => update(i, { refHigh: v })}
                  className="tnum w-15 text-right"
                />
              </li>
            ))}
          </ul>
        </div>
      </div>

      <button
        type="button"
        onClick={() => setRows((prev) => [...prev, emptyRow()])}
        className="pressable mt-2 h-9 w-full rounded-lg border border-line text-xs text-muted"
      >
        行を足す
      </button>

      <input
        type="text"
        aria-label="メモ"
        value={note}
        placeholder="メモ（絶食の有無など、条件が変わると値も変わる）"
        onChange={(e) => setNote(e.target.value)}
        className="mt-3 h-10 w-full rounded-lg border border-line bg-surface-2 px-2 text-sm outline-none focus:border-accent"
      />

      <button
        type="button"
        onClick={save}
        disabled={pending || items.length === 0}
        className="pressable mt-3 h-12 w-full rounded-xl bg-accent text-sm font-bold text-accent-ink disabled:opacity-40"
      >
        {pending ? "登録中…" : items.length === 0 ? "項目を1つ以上入れる" : "登録する"}
      </button>

      {message && (
        <p
          role="status"
          className={`mt-3 rounded-xl border px-3 py-2 text-sm ${
            message.ok
              ? "border-accent/40 bg-accent/10 text-accent"
              : "border-danger/40 bg-danger/10 text-danger"
          }`}
        >
          {message.text}
        </p>
      )}
    </section>
  );
}

function Cell({
  label,
  value,
  placeholder,
  onChange,
  className,
}: {
  label: string;
  value: string;
  placeholder: string;
  onChange: (v: string) => void;
  className: string;
}) {
  return (
    <input
      type="text"
      aria-label={label}
      value={value}
      placeholder={placeholder}
      onChange={(e) => onChange(e.target.value)}
      className={`h-9 min-w-0 rounded-lg border border-line bg-surface-2 px-2 text-sm outline-none focus:border-accent ${className}`}
    />
  );
}
