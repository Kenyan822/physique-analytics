"use client";

import { useState, useTransition } from "react";

import type { Contest, ContestInput } from "@/lib/api/client";
import { parseInput } from "@/lib/number";

import { endOfMonth, sortContests } from "./contest";
import type { ContestResult } from "./contestActions";

type Props = {
  contests: Contest[];
  today: string;
  createContest: (input: ContestInput) => Promise<ContestResult>;
  deleteContest: (id: string) => Promise<{ ok: boolean; message?: string }>;
};

/**
 * 入力中の値。**目標BF% だけ文字列で持つ。**
 * 数値に直しながら保持すると「11.」と打った時点で 11 に丸められ、
 * 続きの小数が打てない
 */
type Draft = {
  heldOn: string;
  category: string;
  targetBfPct: string;
  goal: string;
};

/**
 * 大会の管理（要件 P-04）。
 *
 * ここで登録した大会が、週次レポートのカウントダウンと
 * 必要ペース判定（要件 A-10）の入力になる。**空だとその出力が出ない。**
 */
export function ContestEditor({ contests, today, createContest, deleteContest }: Props) {
  const [list, setList] = useState<Contest[]>(() => sortContests(contests));
  const [draft, setDraft] = useState<Draft>(() => emptyDraft(today));
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [pending, startTransition] = useTransition();

  const targetBfPct = parseInput(draft.targetBfPct, 0);
  const canSave = draft.category.trim() !== "" && targetBfPct > 0;

  function save() {
    if (!canSave) return;
    setMessage(null);

    startTransition(async () => {
      const res = await createContest({
        heldOn: draft.heldOn,
        category: draft.category.trim(),
        targetBfPct,
        goal: draft.goal.trim() === "" ? null : draft.goal.trim(),
      });
      if (!res.ok) {
        setMessage({ ok: false, text: res.message });

        return;
      }
      setList((prev) => sortContests([...prev, res.contest]));
      setDraft(emptyDraft(today));
      setMessage({ ok: true, text: "登録した" });
    });
  }

  function remove(c: Contest) {
    setMessage(null);
    startTransition(async () => {
      const res = await deleteContest(c.id);
      if (!res.ok) {
        setMessage({ ok: false, text: res.message ?? "削除できなかった" });

        return;
      }
      setList((prev) => prev.filter((x) => x.id !== c.id));
    });
  }

  return (
    <section className="rounded-2xl border border-line bg-surface p-4">
      <div className="mb-3 flex items-baseline justify-between gap-2">
        <h2 className="text-sm font-medium">大会</h2>
        <span className="text-[11px] text-muted">残り週数と必要ペースの判定に使う</span>
      </div>

      {list.length === 0 ? (
        <p className="text-sm text-muted">まだ無い。日程が決まる前は月末の日付で入れておく</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {list.map((c) => (
            <li
              key={c.id}
              className="flex items-center gap-2 rounded-xl border border-line bg-surface-2 p-3"
            >
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm">{c.category}</span>
                <span className="tnum block text-xs text-muted">
                  {c.heldOn} / 目標 {c.targetBfPct}%
                  {c.goal && <span className="ml-1">/ {c.goal}</span>}
                </span>
              </span>
              <button
                type="button"
                aria-label={`${c.category}を削除`}
                onClick={() => remove(c)}
                disabled={pending}
                className="pressable shrink-0 rounded-lg px-2 py-1 text-muted disabled:opacity-30"
              >
                ×
              </button>
            </li>
          ))}
        </ul>
      )}

      <div className="mt-3 flex flex-col gap-2 rounded-xl border border-line bg-surface-2 p-3">
        <input
          type="text"
          aria-label="カテゴリ"
          value={draft.category}
          placeholder="カテゴリ（例: スタイリッシュガイ）"
          onChange={(e) => setDraft({ ...draft, category: e.target.value })}
          className="h-10 w-full rounded-lg border border-line bg-surface px-2 text-sm outline-none focus:border-accent"
        />

        <div className="flex flex-wrap items-center gap-2">
          <input
            type="date"
            aria-label="開催日"
            value={draft.heldOn}
            onChange={(e) => setDraft({ ...draft, heldOn: e.target.value })}
            className="tnum h-10 rounded-lg border border-line bg-surface px-2 text-sm outline-none focus:border-accent"
          />
          <label className="flex shrink-0 items-center gap-1.5">
            <input
              type="text"
              inputMode="decimal"
              aria-label="目標体脂肪率"
              value={draft.targetBfPct}
              onChange={(e) => setDraft({ ...draft, targetBfPct: e.target.value })}
              className="tnum h-10 w-16 rounded-lg border border-line bg-surface px-2 text-right text-sm outline-none focus:border-accent"
            />
            <span className="text-[11px] text-muted">% 目標BF</span>
          </label>
        </div>

        <input
          type="text"
          aria-label="位置づけ"
          value={draft.goal}
          placeholder="位置づけ（例: 完走・経験を積む）"
          onChange={(e) => setDraft({ ...draft, goal: e.target.value })}
          className="h-10 w-full rounded-lg border border-line bg-surface px-2 text-sm outline-none focus:border-accent"
        />

        <button
          type="button"
          onClick={save}
          disabled={pending || !canSave}
          className="pressable h-10 w-full rounded-xl border border-line text-sm text-muted disabled:opacity-40"
        >
          {pending ? "登録中…" : "大会を追加"}
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
 * 新規入力の初期値。
 *
 * 開催日を**その月の末日**にしてある。日程が出る前に登録しておけるようにし、
 * 月内で一番遠い日を取ることで必要ペースを過小評価しないため（openapi.yaml の heldOn）。
 */
function emptyDraft(today: string): Draft {
  return { heldOn: endOfMonth(today), category: "", targetBfPct: "11", goal: "" };
}
