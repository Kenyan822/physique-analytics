"use client";

import { useState, useTransition } from "react";

import type { CsvResource } from "@/lib/api/client";

import { RESOURCES, exportFileName, resourceLabel } from "./csv";
import type { ExportResult } from "./actions";

type Props = {
  exportCsv: (resource: CsvResource, from: string, to: string) => Promise<ExportResult>;
};

/**
 * CSV の書き出し（要件 I-02）。
 *
 * `reference/analysis/` の入力形式であり、長期バックアップの正でもある
 * （ADR-0002 / ADR-0011）。サービスが止まってもこれがあれば読める。
 */
export function ExportPanel({ exportCsv }: Props) {
  const [resource, setResource] = useState<CsvResource>("daily");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  function run() {
    setError(null);
    setDone(null);

    startTransition(async () => {
      const res = await exportCsv(resource, from, to);
      if (!res.ok) {
        setError(res.message);

        return;
      }

      const name = exportFileName(resource, from, to);
      // **API のURLに直接リンクしない。** トークンはサーバ側にしか無いので、
      // ブラウザから叩くと 401 になる。本文を受け取ってから保存させる
      const url = URL.createObjectURL(new Blob([res.csv], { type: "text/csv;charset=utf-8" }));
      const a = document.createElement("a");
      a.href = url;
      a.download = name;
      a.click();
      URL.revokeObjectURL(url);
      setDone(name);
    });
  }

  return (
    <section className="rounded-2xl border border-line bg-surface p-4">
      <div className="mb-3 flex items-baseline justify-between gap-2">
        <h2 className="text-sm font-medium">書き出す</h2>
        <span className="text-[11px] text-muted">長期バックアップの正</span>
      </div>

      <div className="flex gap-2 overflow-x-auto">
        {RESOURCES.map((r) => (
          <Chip key={r} active={resource === r} onClick={() => setResource(r)}>
            {resourceLabel(r)}
          </Chip>
        ))}
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        <input
          type="date"
          aria-label="開始日"
          value={from}
          onChange={(e) => setFrom(e.target.value)}
          className="tnum h-10 rounded-lg border border-line bg-surface-2 px-2 text-sm outline-none focus:border-accent"
        />
        <span className="text-xs text-muted">〜</span>
        <input
          type="date"
          aria-label="終了日"
          value={to}
          onChange={(e) => setTo(e.target.value)}
          className="tnum h-10 rounded-lg border border-line bg-surface-2 px-2 text-sm outline-none focus:border-accent"
        />
        <span className="text-[11px] text-muted">空なら全期間</span>
      </div>

      <button
        type="button"
        onClick={run}
        disabled={pending}
        className="pressable mt-3 h-12 w-full rounded-xl border border-line text-sm disabled:opacity-40"
      >
        {pending ? "書き出し中…" : `${exportFileName(resource, from, to)} を保存`}
      </button>

      {done && (
        <p
          role="status"
          className="mt-3 rounded-xl border border-accent/40 bg-accent/10 px-3 py-2 text-sm text-accent"
        >
          {done} を保存した
        </p>
      )}
      {error && (
        <p
          role="alert"
          className="mt-3 rounded-xl border border-danger/40 bg-danger/10 px-3 py-2 text-sm text-danger"
        >
          {error}
        </p>
      )}
    </section>
  );
}

function Chip({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={`pressable shrink-0 rounded-full border px-3.5 py-1.5 text-sm ${
        active
          ? "border-accent bg-accent font-semibold text-accent-ink"
          : "border-line bg-surface-2 text-muted"
      }`}
    >
      {children}
    </button>
  );
}
