"use client";

import { useRef, useState, useTransition } from "react";

import type { MealEstimate } from "@/lib/api/client";

import type { EstimateResult } from "./actions";
import { confidenceLabel, toDraft, type Draft } from "./estimate";

type Props = {
  estimateMeal: (form: FormData) => Promise<EstimateResult>;
  onEstimated: (draft: Draft) => void;
};

/** 補足テキストの上限（openapi.yaml の note: maxLength 500） */
const MAX_NOTE = 500;

/**
 * 写真から PFC を推定する（要件 N-06）。
 *
 * **食品マスタを持たない設計の弱点は「初めて食べるものの入力」**で、そこを埋める。
 * 履歴にあるものは N-02 の候補から選ぶ方が速いので、これは初回専用。
 */
export function EstimatePanel({ estimateMeal, onEstimated }: Props) {
  const [note, setNote] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [result, setResult] = useState<MealEstimate | null>(null);
  const [error, setError] = useState<{ text: string; unavailable: boolean } | null>(null);
  const [pending, startTransition] = useTransition();
  const inputRef = useRef<HTMLInputElement>(null);

  function run() {
    if (!file) return;
    setError(null);
    setResult(null);

    const form = new FormData();
    form.set("image", file, file.name);
    if (note.trim() !== "") form.set("note", note.trim().slice(0, MAX_NOTE));

    startTransition(async () => {
      const res = await estimateMeal(form);
      if (!res.ok) {
        setError({ text: res.message, unavailable: res.unavailable });

        return;
      }
      setResult(res.estimate);
      // **入力欄に入れるだけ。** 確認して直してから記録する
      onEstimated(toDraft(res.estimate));
      setFile(null);
      if (inputRef.current) inputRef.current.value = "";
    });
  }

  return (
    <section className="rounded-2xl border border-line bg-surface p-4">
      <div className="mb-3 flex items-baseline justify-between gap-2">
        <h2 className="text-sm font-medium">写真から推定</h2>
        <span className="text-[11px] text-muted">初めて食べるもの用</span>
      </div>

      <input
        ref={inputRef}
        type="file"
        accept="image/*"
        aria-label="食事の写真"
        onChange={(e) => {
          setFile(e.target.files?.[0] ?? null);
          setError(null);
        }}
        className="w-full text-sm text-muted file:mr-3 file:rounded-lg file:border file:border-line file:bg-surface-2 file:px-3 file:py-2 file:text-sm file:text-ink"
      />

      <input
        type="text"
        aria-label="量の補足"
        value={note}
        maxLength={MAX_NOTE}
        placeholder="鶏むね200g、白米150g"
        onChange={(e) => setNote(e.target.value)}
        className="mt-2 h-10 w-full rounded-lg border border-line bg-surface-2 px-2 text-sm outline-none focus:border-accent"
      />
      {/* 写真だけでは食器のサイズが分からない。量を添えると精度が大きく上がる */}
      <p className="mt-1 text-[11px] text-muted">量を添えると精度が上がる</p>

      <button
        type="button"
        onClick={run}
        disabled={pending || file === null}
        className="pressable mt-3 h-11 w-full rounded-xl border border-line text-sm disabled:opacity-40"
      >
        {pending ? "推定中…" : file ? "推定する" : "写真を選ぶ"}
      </button>

      {result && (
        <div role="status" className="mt-3 rounded-xl border border-line bg-surface-2 p-3">
          <p className="text-sm">
            入力欄に入れた。<span className="text-muted">確認して直してから記録する</span>
          </p>
          <p className="mt-1 text-[11px] text-muted">
            {confidenceLabel(result.confidence)}
            {result.note && ` / ${result.note}`}
          </p>
        </div>
      )}

      {error && (
        <p
          role="alert"
          className={`mt-3 rounded-xl border px-3 py-2 text-sm ${
            error.unavailable
              ? "border-warn/40 bg-warn/10 text-warn"
              : "border-danger/40 bg-danger/10 text-danger"
          }`}
        >
          {error.text}
        </p>
      )}
    </section>
  );
}
