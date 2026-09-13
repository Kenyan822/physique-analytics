"use client";

import { useEffect, useRef, useState } from "react";

import type { MealSuggestion } from "@/lib/api/client";

type Props = {
  loadSuggestions: (q: string) => Promise<MealSuggestion[]>;
  onPick: (s: MealSuggestion) => void;
  onClose: () => void;
};

/**
 * 過去の記録から選ぶ（要件 N-02）。
 *
 * **食品マスタは無い。** 履歴がマスタなので、頻度順に並べて上から選べば
 * 2回目以降の入力は1タップで終わる。
 */
export function MealPicker({ loadSuggestions, onPick, onClose }: Props) {
  const [query, setQuery] = useState("");
  const [items, setItems] = useState<MealSuggestion[]>([]);
  const [loading, setLoading] = useState(true);
  const panelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let cancelled = false;
    // 打つたびに投げない。候補が変わるのは一呼吸置いてからでよい
    const id = setTimeout(() => {
      setLoading(true);
      loadSuggestions(query)
        .then((res) => {
          if (!cancelled) setItems(res);
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });
    }, 200);

    return () => {
      cancelled = true;
      clearTimeout(id);
    };
  }, [query, loadSuggestions]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        onClose();

        return;
      }
      if (e.key !== "Tab") return;

      const focusable = panelRef.current?.querySelectorAll<HTMLElement>(
        'input, button, select, [href], [tabindex]:not([tabindex="-1"])',
      );
      if (!focusable?.length) return;

      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);

    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex flex-col bg-bg sm:items-center sm:justify-center sm:bg-black/70 sm:p-6"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label="過去の記録から選ぶ"
        className="flex min-h-0 flex-1 flex-col sm:h-[min(36rem,85vh)] sm:w-full sm:max-w-lg sm:flex-none sm:overflow-hidden sm:rounded-3xl sm:border sm:border-line sm:shadow-2xl"
      >
        <div className="flex items-center gap-2 border-b border-line bg-bg px-4 py-3">
          <input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="食べたものを探す"
            className="h-11 flex-1 rounded-xl border border-line bg-surface px-3 text-base outline-none focus:border-accent"
          />
          <button
            type="button"
            onClick={onClose}
            className="pressable h-11 shrink-0 rounded-xl px-3 text-sm text-muted"
          >
            閉じる
          </button>
        </div>

        <ul className="flex-1 overflow-y-auto bg-bg pb-8">
          {!loading && items.length === 0 && (
            <li className="px-4 py-8 text-center text-sm text-muted">
              {query ? "見つからない。下の「新しく入力」から記録する" : "まだ記録が無い"}
            </li>
          )}
          {items.map((s) => (
            <li key={s.name}>
              <button
                type="button"
                onClick={() => onPick(s)}
                className="pressable flex w-full items-center justify-between gap-3 border-b border-line/60 px-4 py-3.5 text-left"
              >
                <span className="min-w-0">
                  <span className="block truncate text-base">{s.name}</span>
                  <span className="tnum block text-xs text-muted">
                    {s.kcal != null && `${s.kcal} kcal`}
                    {s.proteinG != null && ` / P ${s.proteinG}g`}
                    {s.qty && ` / ${s.qty}`}
                  </span>
                </span>
                <span className="tnum shrink-0 rounded-full bg-surface-2 px-2.5 py-1 text-xs text-muted">
                  {s.count} 回
                </span>
              </button>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
