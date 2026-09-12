"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { Exercise, MuscleGroup } from "@/lib/api/client";

type Props = {
  exercises: Exercise[];
  selected: Exercise | undefined;
  /** その種目で今日すでに記録したセット数 */
  doneCount: (id: string) => number;
  onSelect: (id: string) => void;
};

/**
 * 種目の選択（要件 T-01）。
 *
 * **49種目を `<select>` で出すとジムで使えない。** 汗で滑る指で
 * 長いリストをスクロールすることになる。検索と部位での絞り込みを付け、
 * 候補を数件まで減らしてから選ばせる。
 */
export function ExercisePicker({ exercises, selected, doneCount, onSelect }: Props) {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);

  /*
   * 閉じたらこのボタンへフォーカスを戻す。シート側で「開く前にどこに
   * いたか」を退避すると、`autoFocus` が先に走っていて検索欄を拾う
   */
  const close = useCallback(() => {
    setOpen(false);
    triggerRef.current?.focus();
  }, []);

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        onClick={() => setOpen(true)}
        className="pressable flex w-full items-center justify-between gap-3 rounded-2xl border border-line bg-surface px-4 py-4 text-left"
      >
        <span className="min-w-0">
          <span className="block text-xs text-muted">種目</span>
          {selected ? (
            <span className="block truncate text-lg font-semibold">{selected.name}</span>
          ) : (
            <span className="block text-lg text-muted">選ぶ</span>
          )}
        </span>
        <span className="shrink-0 text-muted" aria-hidden>
          ›
        </span>
      </button>

      {open && (
        <PickerSheet
          exercises={exercises}
          doneCount={doneCount}
          onClose={close}
          onSelect={(id) => {
            onSelect(id);
            close();
          }}
        />
      )}
    </>
  );
}

function PickerSheet({
  exercises,
  doneCount,
  onSelect,
  onClose,
}: {
  exercises: Exercise[];
  doneCount: (id: string) => number;
  onSelect: (id: string) => void;
  onClose: () => void;
}) {
  const [query, setQuery] = useState("");
  const [group, setGroup] = useState<MuscleGroup | null>(null);

  // 候補に出ている種目の部位だけを並べる。使わない部位のチップは邪魔
  const groups = useMemo(() => {
    const seen = new Set<MuscleGroup>();
    const out: MuscleGroup[] = [];
    for (const e of exercises) {
      if (!seen.has(e.muscleGroup)) {
        seen.add(e.muscleGroup);
        out.push(e.muscleGroup);
      }
    }
    return out;
  }, [exercises]);

  const shown = useMemo(() => {
    const q = query.trim();

    return exercises.filter((e) => {
      if (group && e.muscleGroup !== group) return false;
      if (!q) return true;
      // かな入力の途中でも引っかかるよう、部分一致だけにする
      return e.name.includes(q) || e.muscleGroup.includes(q);
    });
  }, [exercises, query, group]);

  const panelRef = useRef<HTMLDivElement>(null);

  /*
   * Esc で閉じ、Tab を内側に閉じ込める。`aria-modal` はフォーカスを
   * 拘束しないので、これが無いと Tab で背後の画面へ抜けて戻れなくなる
   */
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
    /*
     * 狭い画面は全面シート、広い画面は中央のダイアログ。
     * 49種目を全面に出すのはスマホでは正しいが、デスクトップでは
     * 画面全部が置き換わって「どこに戻るのか」が分からなくなる
     */
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
        aria-label="種目を選ぶ"
        className="flex min-h-0 flex-1 flex-col sm:h-[min(36rem,85vh)] sm:w-full sm:max-w-lg sm:flex-none sm:overflow-hidden sm:rounded-3xl sm:border sm:border-line sm:shadow-2xl"
      >
        <div className="flex items-center gap-2 border-b border-line bg-bg px-4 py-3">
          <input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="種目を探す"
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

        <div className="flex gap-2 overflow-x-auto border-b border-line bg-bg px-4 py-3">
          <Chip active={group === null} onClick={() => setGroup(null)}>
            すべて
          </Chip>
          {groups.map((g) => (
            <Chip key={g} active={group === g} onClick={() => setGroup(group === g ? null : g)}>
              {g}
            </Chip>
          ))}
        </div>

        <ul className="flex-1 overflow-y-auto bg-bg pb-8">
          {shown.length === 0 && (
            <li className="px-4 py-8 text-center text-sm text-muted">見つからない</li>
          )}
          {shown.map((e) => {
            const n = doneCount(e.id);

            return (
              <li key={e.id}>
                <button
                  type="button"
                  onClick={() => onSelect(e.id)}
                  className="pressable flex w-full items-center justify-between gap-3 border-b border-line/60 px-4 py-4 text-left"
                >
                  <span className="min-w-0">
                    <span className="block truncate text-base">{e.name}</span>
                    <span className="block text-xs text-muted">{e.muscleGroup}</span>
                  </span>
                  {n > 0 && (
                    <span className="tnum shrink-0 rounded-full bg-accent px-2.5 py-1 text-xs font-semibold text-accent-ink">
                      {n} セット
                    </span>
                  )}
                </button>
              </li>
            );
          })}
        </ul>
      </div>
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
      className={`pressable shrink-0 rounded-full border px-3 py-1.5 text-sm ${
        active
          ? "border-accent bg-accent text-accent-ink font-semibold"
          : "border-line bg-surface text-muted"
      }`}
    >
      {children}
    </button>
  );
}
