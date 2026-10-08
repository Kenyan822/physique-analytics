"use client";

import { useState, useTransition } from "react";

import type { ManualTargetEntry } from "@/lib/api/client";

import { entryStatuses, type EntryStatus } from "./manualTargets";
import type { TargetResult } from "./targetActions";

type Props = {
  entries: ManualTargetEntry[];
  /** JST の今日（YYYY-MM-DD） */
  today: string;
  remove: (id: string) => Promise<TargetResult>;
};

const LABEL: Record<EntryStatus, string> = {
  current: "適用中",
  scheduled: "予約",
  past: "",
};

/**
 * 手動目標の履歴（要件 N-05）。
 *
 * **過去日の残量はその日に適用されていた目標で出る。** 目標を変えても前の値は
 * ここに残る。消すと、その期間は1つ前の目標（無ければ自動計算）で評価される。
 */
export function TargetHistory({ entries, today, remove }: Props) {
  const [message, setMessage] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  if (entries.length === 0) return null;

  const statuses = entryStatuses(entries, today);

  function del(id: string) {
    setMessage(null);
    startTransition(async () => {
      const res = await remove(id);
      if (!res.ok) setMessage(res.message);
    });
  }

  return (
    <section className="rounded-2xl border border-line bg-surface p-4">
      <h2 className="mb-3 text-sm font-medium">目標の履歴</h2>

      <ul className="divide-y divide-line">
        {entries.map((e) => (
          <li key={e.id} className="flex items-center gap-3 py-2 text-sm">
            <span className="tnum w-24 shrink-0">{e.startsOn}〜</span>
            <span className="tnum flex-1 text-muted">
              P{e.proteinG} F{e.fatG} C{e.carbG} · {e.kcal} kcal
            </span>
            <span className="w-12 text-[11px] text-accent">{LABEL[statuses[e.id]]}</span>
            <button
              type="button"
              onClick={() => del(e.id)}
              disabled={pending}
              aria-label={`${e.startsOn} 開始の目標を消す`}
              className="h-9 rounded-lg border border-line px-3 text-xs disabled:opacity-40"
            >
              消す
            </button>
          </li>
        ))}
      </ul>

      {message && <p className="mt-2 text-xs text-danger">{message}</p>}
    </section>
  );
}
