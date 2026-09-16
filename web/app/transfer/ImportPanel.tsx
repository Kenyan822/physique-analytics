"use client";

import { useRef, useState, useTransition } from "react";

import type { CsvResource } from "@/lib/api/client";

import { RESOURCES, columnsOf, resourceLabel } from "./csv";
import type { ImportOutcome } from "./actions";

type Props = { importCsv: (form: FormData) => Promise<ImportOutcome> };

type OnDuplicate = "skip" | "overwrite";

/**
 * CSV の取り込み（要件 I-01 / I-03）。
 *
 * アプリ完成前に CSV で記録した期間を取り込むための入口。
 * **1行のミスで全部止めない**ので、落ちた行は行番号つきで出す。
 */
export function ImportPanel({ importCsv }: Props) {
  const [resource, setResource] = useState<CsvResource>("daily");
  const [onDuplicate, setOnDuplicate] = useState<OnDuplicate>("skip");
  const [file, setFile] = useState<File | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [outcome, setOutcome] = useState<ImportOutcome | null>(null);
  const [pending, startTransition] = useTransition();
  const inputRef = useRef<HTMLInputElement>(null);

  function run() {
    if (!file) return;
    setConfirming(false);
    setOutcome(null);

    const form = new FormData();
    form.set("resource", resource);
    form.set("onDuplicate", onDuplicate);
    form.set("file", file, file.name);

    startTransition(async () => {
      setOutcome(await importCsv(form));
      setFile(null);
      if (inputRef.current) inputRef.current.value = "";
    });
  }

  return (
    <section className="rounded-2xl border border-line bg-surface p-4">
      <div className="mb-3 flex items-baseline justify-between gap-2">
        <h2 className="text-sm font-medium">取り込む</h2>
        <span className="text-[11px] text-muted">アプリ以前の記録</span>
      </div>

      <div className="flex gap-2 overflow-x-auto">
        {RESOURCES.map((r) => (
          <Chip key={r} active={resource === r} onClick={() => setResource(r)}>
            {resourceLabel(r)}
          </Chip>
        ))}
      </div>

      {/* 手で書いた CSV を直せるよう、期待する列をその場で出す */}
      <p className="tnum mt-3 overflow-x-auto whitespace-nowrap rounded-lg bg-surface-2 px-2 py-1.5 text-[11px] text-muted">
        {columnsOf(resource).join(",")}
      </p>

      <input
        ref={inputRef}
        type="file"
        accept=".csv,text/csv"
        aria-label="CSV ファイル"
        onChange={(e) => {
          setFile(e.target.files?.[0] ?? null);
          setOutcome(null);
        }}
        className="mt-3 w-full text-sm text-muted file:mr-3 file:rounded-lg file:border file:border-line file:bg-surface-2 file:px-3 file:py-2 file:text-sm file:text-ink"
      />

      <div className="mt-3 flex flex-wrap items-center gap-2">
        <span className="text-[11px] text-muted">同じ日・同じ種目があったら</span>
        <Chip active={onDuplicate === "skip"} onClick={() => setOnDuplicate("skip")}>
          飛ばす
        </Chip>
        <Chip active={onDuplicate === "overwrite"} onClick={() => setOnDuplicate("overwrite")}>
          上書きする
        </Chip>
      </div>

      {confirming ? (
        /* 上書きは既存の記録を書き換える。押し間違いを一段挟んで止める */
        <div className="mt-3 rounded-xl border border-warn/40 bg-warn/10 p-3">
          <p className="text-sm text-warn">
            {file?.name} を{resourceLabel(resource)}として取り込む。
            {onDuplicate === "overwrite" && (
              <strong className="font-semibold">既にある記録は上書きされる。</strong>
            )}
          </p>
          <div className="mt-2 flex gap-2">
            <button
              type="button"
              onClick={() => setConfirming(false)}
              className="pressable h-10 flex-1 rounded-xl border border-line text-sm text-muted"
            >
              やめる
            </button>
            <button
              type="button"
              onClick={run}
              className="pressable h-10 flex-1 rounded-xl bg-accent text-sm font-bold text-accent-ink"
            >
              取り込む
            </button>
          </div>
        </div>
      ) : (
        <button
          type="button"
          onClick={() => setConfirming(true)}
          disabled={pending || file === null}
          className="pressable mt-3 h-12 w-full rounded-xl bg-accent text-sm font-bold text-accent-ink disabled:opacity-40"
        >
          {pending ? "取り込み中…" : file ? "内容を確認する" : "ファイルを選ぶ"}
        </button>
      )}

      {outcome && <Outcome outcome={outcome} />}
    </section>
  );
}

function Outcome({ outcome }: { outcome: ImportOutcome }) {
  if (!outcome.ok) {
    return (
      <p
        role="alert"
        className="mt-3 rounded-xl border border-danger/40 bg-danger/10 px-3 py-2 text-sm text-danger"
      >
        {outcome.message}
      </p>
    );
  }

  const { imported, skipped, errors } = outcome.result;

  return (
    <div role="status" className="mt-3 rounded-xl border border-line bg-surface-2 p-3">
      <p className="tnum text-sm">
        取り込み <span className="font-semibold text-accent">{imported}</span> 件 / 飛ばし {skipped}{" "}
        件 / エラー {errors.length} 件
      </p>
      {errors.length > 0 && (
        <ul className="tnum mt-2 flex max-h-48 flex-col gap-1 overflow-y-auto text-xs text-warn">
          {errors.map((e, i) => (
            <li key={`${e.line}-${i}`}>
              {e.line} 行目: {e.message}
            </li>
          ))}
        </ul>
      )}
    </div>
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
