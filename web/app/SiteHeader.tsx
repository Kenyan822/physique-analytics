"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useCallback, useEffect, useRef, useState, useTransition } from "react";

import { NAV_ITEMS, isCurrent } from "@/lib/nav/items";

import { logout } from "./login/actions";

type Props = {
  /** 画面名。見出しとして出す */
  title: string;
  /** 日付など、見出しに添える情報 */
  subtitle?: string;
};

/**
 * 全画面で共通のヘッダー。
 *
 * **どの画面からでも他の画面へ行けるようにする。** 以前はナビがトップにしか
 * 無く、食事から体組成へ移るのに一度トップへ戻る必要があった。
 *
 * 広い画面は横並び、狭い画面はハンバーガー。**「記録する」だけは常に見せる** ——
 * 一番よく使う操作をメニューの中に隠さない。
 */
export function SiteHeader({ title, subtitle }: Props) {
  const pathname = usePathname();
  const [open, setOpen] = useState(false);

  return (
    <header className="sticky top-0 z-30 border-b border-line bg-bg/95 backdrop-blur">
      <div className="mx-auto flex w-full max-w-6xl items-center gap-3 px-4 py-3 lg:px-6">
        <h1 className="flex min-w-0 shrink items-baseline gap-2">
          <span className="truncate text-lg font-semibold lg:text-xl">{title}</span>
          {subtitle && <span className="tnum shrink-0 text-sm text-muted">{subtitle}</span>}
        </h1>

        {/* 広い画面は横並び。狭い画面ではメニューに入れる */}
        <nav aria-label="画面の切り替え" className="ml-auto hidden items-center gap-1 lg:flex">
          {NAV_ITEMS.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              aria-current={isCurrent(pathname, item.href) ? "page" : undefined}
              className={`pressable rounded-full px-3 py-1.5 text-sm ${
                isCurrent(pathname, item.href)
                  ? "bg-surface-2 font-semibold text-ink"
                  : "text-muted hover:text-ink"
              }`}
            >
              {item.label}
            </Link>
          ))}
        </nav>

        <div className="ml-auto flex shrink-0 items-center gap-2 lg:ml-0">
          {pathname !== "/log" && (
            <Link
              href="/log"
              className="pressable rounded-full bg-accent px-4 py-2 text-sm font-bold text-accent-ink"
            >
              記録する
            </Link>
          )}
          <MenuButton open={open} onToggle={() => setOpen((v) => !v)} />
        </div>
      </div>

      {open && <MenuSheet pathname={pathname} onClose={() => setOpen(false)} />}
    </header>
  );
}

function MenuButton({ open, onToggle }: { open: boolean; onToggle: () => void }) {
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-expanded={open}
      aria-controls="site-menu"
      aria-label={open ? "メニューを閉じる" : "メニューを開く"}
      className="pressable flex h-10 w-10 shrink-0 items-center justify-center rounded-full border border-line lg:h-9 lg:w-9"
    >
      {/* 3本線。開いているときは × に替えて状態を示す */}
      <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden className="text-ink">
        {open ? (
          <path d="M3 3 L15 15 M15 3 L3 15" stroke="currentColor" strokeWidth="1.8" fill="none" />
        ) : (
          <path
            d="M2 4.5h14 M2 9h14 M2 13.5h14"
            stroke="currentColor"
            strokeWidth="1.8"
            fill="none"
          />
        )}
      </svg>
    </button>
  );
}

/**
 * メニュー本体。
 *
 * Esc で閉じ、Tab を内側に閉じ込める。`aria-modal` はフォーカスを拘束しないので、
 * これが無いと Tab で背後の画面へ抜けて戻れなくなる（ExercisePicker と同じ理由）。
 */
function MenuSheet({ pathname, onClose }: { pathname: string; onClose: () => void }) {
  const panelRef = useRef<HTMLDivElement>(null);
  const [pending, startTransition] = useTransition();

  const trap = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        onClose();

        return;
      }
      if (e.key !== "Tab") return;

      const focusable = panelRef.current?.querySelectorAll<HTMLElement>(
        'a, button, [tabindex]:not([tabindex="-1"])',
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
    },
    [onClose],
  );

  useEffect(() => {
    document.addEventListener("keydown", trap);

    return () => document.removeEventListener("keydown", trap);
  }, [trap]);

  return (
    <div
      id="site-menu"
      ref={panelRef}
      role="dialog"
      aria-modal="true"
      aria-label="メニュー"
      className="border-t border-line bg-bg"
    >
      <nav aria-label="すべての画面" className="mx-auto w-full max-w-6xl px-4 py-2 lg:px-6">
        <ul className="flex flex-col">
          {NAV_ITEMS.map((item) => (
            <li key={item.href}>
              <Link
                href={item.href}
                onClick={onClose}
                aria-current={isCurrent(pathname, item.href) ? "page" : undefined}
                className={`pressable flex items-baseline justify-between gap-3 border-b border-line/50 py-3 ${
                  isCurrent(pathname, item.href) ? "font-semibold text-accent" : ""
                }`}
              >
                <span className="shrink-0">{item.label}</span>
                <span className="min-w-0 truncate text-right text-xs text-muted">{item.hint}</span>
              </Link>
            </li>
          ))}
        </ul>

        <button
          type="button"
          onClick={() => startTransition(() => logout())}
          disabled={pending}
          className="pressable my-3 h-11 w-full rounded-xl border border-line text-sm text-muted disabled:opacity-40"
        >
          {pending ? "ログアウト中…" : "ログアウト"}
        </button>
      </nav>
    </div>
  );
}
