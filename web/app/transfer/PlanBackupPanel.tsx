"use client";

import { useRef, useState, useTransition } from "react";

import type { PlanExport, PlanImport } from "./actions";
import { backupFileName, parseBackup, toBackup } from "./plan";

type Props = {
  exportPlan: () => Promise<PlanExport>;
  importPlan: (
    plan: Parameters<typeof toBackup>[0],
    blocks: Parameters<typeof toBackup>[1],
  ) => Promise<PlanImport>;
};

/**
 * 計画の設定のバックアップ（要件 I-02 / ADR-0015）。
 *
 * **記録の CSV には設定が含まれない。** DB を消すと3年計画の設計値が消える。
 * Supabase 無料枠の自動バックアップは7日しか保持しない。
 */
export function PlanBackupPanel({ exportPlan, importPlan }: Props) {
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [confirming, setConfirming] = useState<{ name: string; text: string } | null>(null);
  const [pending, startTransition] = useTransition();
  const inputRef = useRef<HTMLInputElement>(null);

  function save() {
    setMessage(null);
    startTransition(async () => {
      const res = await exportPlan();
      if (!res.ok) {
        setMessage({ ok: false, text: res.message });

        return;
      }

      const backup = toBackup(res.plan, res.blocks);
      const name = backupFileName(backup.exportedAt);
      const url = URL.createObjectURL(
        new Blob([JSON.stringify(backup, null, 2)], { type: "application/json" }),
      );
      const a = document.createElement("a");
      a.href = url;
      a.download = name;
      a.click();
      URL.revokeObjectURL(url);
      setMessage({ ok: true, text: `${name} を保存した` });
    });
  }

  async function pick(file: File) {
    setMessage(null);
    const parsed = parseBackup(await file.text());
    if (!parsed.ok) {
      setMessage({ ok: false, text: parsed.message });

      return;
    }
    setConfirming({ name: file.name, text: JSON.stringify(parsed.backup) });
  }

  function restore() {
    if (!confirming) return;
    const parsed = parseBackup(confirming.text);
    setConfirming(null);
    if (!parsed.ok) return;

    startTransition(async () => {
      const res = await importPlan(parsed.backup.plan, parsed.backup.blocks);
      if (inputRef.current) inputRef.current.value = "";
      setMessage(
        res.ok
          ? { ok: true, text: "設定を戻した" }
          : { ok: false, text: res.message ?? "戻せなかった" },
      );
    });
  }

  return (
    <section className="rounded-2xl border border-line bg-surface p-4">
      <div className="mb-3 flex items-baseline justify-between gap-2">
        <h2 className="text-sm font-medium">計画の設定</h2>
        <span className="text-[11px] text-muted">記録とは別に保存する</span>
      </div>

      <p className="mb-3 text-[11px] text-muted">
        フェーズ・栄養・MEV/MRV・起点・ブロック。消えると計画を組み直すことになる。
      </p>

      <button
        type="button"
        onClick={save}
        disabled={pending}
        className="pressable h-11 w-full rounded-xl border border-line text-sm disabled:opacity-40"
      >
        {pending ? "処理中…" : "設定を保存"}
      </button>

      <input
        ref={inputRef}
        type="file"
        accept=".json,application/json"
        aria-label="設定のバックアップ"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) void pick(f);
        }}
        className="mt-3 w-full text-sm text-muted file:mr-3 file:rounded-lg file:border file:border-line file:bg-surface-2 file:px-3 file:py-2 file:text-sm file:text-ink"
      />

      {confirming && (
        /* 読み込みは設定をまるごと置き換える。押し間違いを一段挟んで止める */
        <div className="mt-3 rounded-xl border border-warn/40 bg-warn/10 p-3">
          <p className="text-sm text-warn">
            {confirming.name} で
            <strong className="font-semibold">いまの設定をまるごと置き換える。</strong>
          </p>
          <div className="mt-2 flex gap-2">
            <button
              type="button"
              onClick={() => {
                setConfirming(null);
                if (inputRef.current) inputRef.current.value = "";
              }}
              className="pressable h-10 flex-1 rounded-xl border border-line text-sm text-muted"
            >
              やめる
            </button>
            <button
              type="button"
              onClick={restore}
              className="pressable h-10 flex-1 rounded-xl bg-accent text-sm font-bold text-accent-ink"
            >
              置き換える
            </button>
          </div>
        </div>
      )}

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
