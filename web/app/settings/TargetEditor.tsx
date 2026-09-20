"use client";

import { useState, useTransition } from "react";

import type { ManualTargets } from "@/lib/api/client";

import { kcalFromMacros, toManualTargets, type TargetDraft } from "./manualTargets";
import type { TargetResult } from "./targetActions";

type Props = {
  /** いま保存されている手動目標。null なら自動計算（A-02）が使われている */
  current: ManualTargets | null;
  save: (input: ManualTargets) => Promise<TargetResult>;
  clear: () => Promise<TargetResult>;
};

/**
 * 手で決めた摂取目標（要件 N-05）。
 *
 * **計画のフォームと分けてある。** あちらは `PUT /v1/plan` でまるごと
 * 置き換える作りで、混ぜると基準値が消える（#127 で踏んだ）。
 */
export function TargetEditor({ current, save, clear }: Props) {
  const [draft, setDraft] = useState<TargetDraft>(() => ({
    proteinG: current?.proteinG?.toString() ?? "",
    fatG: current?.fatG?.toString() ?? "",
    carbG: current?.carbG?.toString() ?? "",
  }));
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [pending, startTransition] = useTransition();

  const parsed = toManualTargets(draft);
  const kcal = parsed ? kcalFromMacros(parsed.proteinG, parsed.fatG, parsed.carbG) : null;

  function submit() {
    if (!parsed) return;
    setMessage(null);
    startTransition(async () => {
      const res = await save(parsed);
      setMessage(res.ok ? { ok: true, text: "保存した" } : { ok: false, text: res.message });
    });
  }

  function reset() {
    setMessage(null);
    startTransition(async () => {
      const res = await clear();
      if (res.ok) {
        setDraft({ proteinG: "", fatG: "", carbG: "" });
        setMessage({ ok: true, text: "自動計算に戻した" });
      } else {
        setMessage({ ok: false, text: res.message });
      }
    });
  }

  return (
    <section className="rounded-2xl border border-line bg-surface p-4">
      <div className="mb-3 flex items-baseline justify-between">
        <h2 className="text-sm font-medium">1日の目標</h2>
        <span className="text-[11px] text-muted">
          {current ? "手動で決めた値を使っている" : "体重トレンドから自動で決まる"}
        </span>
      </div>

      <div className="flex flex-wrap gap-3">
        <Macro label="P" value={draft.proteinG} onChange={(v) => setDraft({ ...draft, proteinG: v })} />
        <Macro label="F" value={draft.fatG} onChange={(v) => setDraft({ ...draft, fatG: v })} />
        <Macro label="C" value={draft.carbG} onChange={(v) => setDraft({ ...draft, carbG: v })} />
        <div className="flex flex-col justify-end">
          <span className="mb-1 block text-xs text-muted">カロリー</span>
          <span className="tnum flex h-11 items-center text-base">
            {kcal === null ? "— kcal" : `${kcal} kcal`}
          </span>
        </div>
      </div>

      <p className="mt-2 text-[11px] text-muted">
        {/* **3つで1組。** 1つ欠けた目標は意味を成さない */}
        P・F・C を3つとも入れる。kcal は自動で出る（4/9/4）
      </p>

      <div className="mt-3 flex items-center gap-2">
        <button
          type="button"
          onClick={submit}
          disabled={parsed === null || pending}
          className="h-11 rounded-xl bg-accent px-4 text-sm font-medium text-bg disabled:opacity-40"
        >
          {pending ? "保存中…" : "保存"}
        </button>

        {current && (
          <button
            type="button"
            onClick={reset}
            disabled={pending}
            className="h-11 rounded-xl border border-line px-4 text-sm disabled:opacity-40"
          >
            自動計算に戻す
          </button>
        )}
      </div>

      {message && (
        <p className={`mt-2 text-xs ${message.ok ? "text-muted" : "text-danger"}`}>{message.text}</p>
      )}
    </section>
  );
}

function Macro({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs text-muted">{label}（g）</span>
      <input
        type="text"
        inputMode="decimal"
        aria-label={label}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="tnum h-11 w-24 rounded-xl border border-line bg-surface-2 px-3 text-base outline-none focus:border-accent"
      />
    </label>
  );
}
