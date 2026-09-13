"use client";

import { useState, useTransition } from "react";

import type { PlanBlock } from "@/lib/api/client";
import { parseInput } from "@/lib/number";
import type { SaveBlocksResult } from "./actions";
import { findBlockError, readSigned, totalMonths } from "./blocks";

type Props = {
  blocks: PlanBlock[];
  saveBlocks: (items: PlanBlock[]) => Promise<SaveBlocksResult>;
};

/** 3年計画の想定。これを下回っていると最後まで引けていない */
const PLAN_MONTHS = 36;

const NEW_BLOCK: PlanBlock = {
  name: "",
  months: 6,
  lbmDeltaKgPerMonth: 0.3,
  bodyfatPctEnd: 15,
  focus: null,
};

/**
 * 計画ブロックの編集（要件 P-02）。
 *
 * ブロックは**並び順が意味を持つ**。上から順に積まれて月次目標になるので、
 * 日付ではなく順番で管理する。
 */
export function PlanBlocksEditor({ blocks, saveBlocks }: Props) {
  const [items, setItems] = useState<PlanBlock[]>(blocks);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [pending, startTransition] = useTransition();

  const error = findBlockError(items);
  const months = totalMonths(items);

  function update(index: number, patch: Partial<PlanBlock>) {
    setItems((prev) => prev.map((b, i) => (i === index ? { ...b, ...patch } : b)));
  }

  function save() {
    if (error) return;
    setMessage(null);

    startTransition(async () => {
      const res = await saveBlocks(items);
      if (!res.ok) {
        setMessage({ ok: false, text: res.message });

        return;
      }
      setItems(res.items);
      setMessage({ ok: true, text: "保存した。月次目標を引き直すには再読み込みする" });
    });
  }

  return (
    <section className="rounded-2xl border border-line bg-surface p-4">
      <div className="mb-3 flex items-baseline justify-between gap-2">
        <h2 className="text-sm font-medium">計画ブロック</h2>
        <span className="tnum text-[11px] text-muted">
          計 {months}ヶ月
          {months < PLAN_MONTHS && (
            <span className="text-warn"> / 3年に {PLAN_MONTHS - months}ヶ月足りない</span>
          )}
        </span>
      </div>

      <ul className="flex flex-col gap-2">
        {items.map((b, i) => (
          <li
            key={i}
            className={`rounded-xl border bg-surface-2 p-3 ${
              error?.index === i ? "border-danger" : "border-line"
            }`}
          >
            <div className="flex items-center gap-2">
              <span className="tnum w-5 shrink-0 text-center text-xs text-muted">{i + 1}</span>
              <input
                type="text"
                aria-label={`ブロック${i + 1}の名前`}
                value={b.name}
                placeholder="Y1 増量"
                onChange={(e) => update(i, { name: e.target.value })}
                className="h-10 min-w-0 flex-1 rounded-lg border border-line bg-surface px-2 text-sm outline-none focus:border-accent"
              />
              <button
                type="button"
                aria-label={`ブロック${i + 1}を削除`}
                onClick={() => setItems((prev) => prev.filter((_, x) => x !== i))}
                disabled={pending}
                className="pressable shrink-0 rounded-lg px-2 py-1 text-muted disabled:opacity-30"
              >
                ×
              </button>
            </div>

            <div className="mt-2 flex flex-wrap items-center gap-3 pl-7">
              <Num
                label={`ブロック${i + 1}の月数`}
                unit="ヶ月"
                value={b.months}
                onChange={(v) => update(i, { months: Math.round(v) })}
              />
              <Num
                label={`ブロック${i + 1}のLBM増減`}
                unit="kg/月 LBM"
                value={b.lbmDeltaKgPerMonth}
                signed
                onChange={(v) => update(i, { lbmDeltaKgPerMonth: v })}
              />
              <Num
                label={`ブロック${i + 1}の終了時体脂肪率`}
                unit="% 終了時"
                value={b.bodyfatPctEnd}
                onChange={(v) => update(i, { bodyfatPctEnd: v })}
              />
            </div>

            <input
              type="text"
              aria-label={`ブロック${i + 1}でやること`}
              value={b.focus ?? ""}
              placeholder="やること（数字だけでは行動が決まらない）"
              onChange={(e) => update(i, { focus: e.target.value === "" ? null : e.target.value })}
              className="mt-2 ml-7 h-10 w-[calc(100%-1.75rem)] rounded-lg border border-line bg-surface px-2 text-sm outline-none focus:border-accent"
            />
          </li>
        ))}
      </ul>

      {error && (
        <p role="alert" className="mt-2 text-xs text-danger">
          {error.index + 1}件目: {error.message}
        </p>
      )}

      <div className="mt-3 flex gap-2">
        <button
          type="button"
          onClick={() => setItems((prev) => [...prev, { ...NEW_BLOCK }])}
          disabled={pending}
          className="pressable h-10 flex-1 rounded-xl border border-line text-sm text-muted disabled:opacity-40"
        >
          ブロックを追加
        </button>
        <button
          type="button"
          onClick={save}
          disabled={pending || error !== null}
          className="pressable h-10 flex-1 rounded-xl bg-accent text-sm font-bold text-accent-ink disabled:opacity-40"
        >
          {pending ? "保存中…" : "保存"}
        </button>
      </div>

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

/**
 * 入力中だけ文字列を持つ数値欄。
 *
 * 数値に直しながら持つと「0.」と打った時点で 0 に丸められ、続きの小数が
 * 打てない。LBM の増減は 0.3 のような値なので、これが無いと入力できない。
 */
function Num({
  label,
  unit,
  value,
  signed,
  onChange,
}: {
  label: string;
  unit: string;
  value: number;
  signed?: boolean;
  onChange: (v: number) => void;
}) {
  const [draft, setDraft] = useState<string | null>(null);

  return (
    <label className="flex shrink-0 items-center gap-1.5">
      <input
        type="text"
        inputMode="decimal"
        aria-label={label}
        value={draft ?? String(value)}
        onFocus={(e) => {
          setDraft(String(value));
          e.currentTarget.select();
        }}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => {
          if (draft !== null)
            onChange(signed ? readSigned(draft, value) : parseInput(draft, value));
          setDraft(null);
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") e.currentTarget.blur();
        }}
        className="tnum h-10 w-16 rounded-lg border border-line bg-surface px-2 text-right text-sm outline-none focus:border-accent"
      />
      <span className="text-[11px] text-muted">{unit}</span>
    </label>
  );
}
