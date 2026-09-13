"use client";

import { useState, useTransition } from "react";

import type { MonthlyTargets } from "@/lib/api/client";

import type { TargetsResult } from "./actions";

type Baseline = "configured" | "measured";

type Props = {
  /** 初期表示（設定値を起点にしたもの）。出せないときは null */
  initial: MonthlyTargets | null;
  /** 初期表示が出せなかった理由 */
  initialMessage: string | null;
  loadMonthlyTargets: (baseline: Baseline) => Promise<TargetsResult>;
};

/**
 * 月次目標の表（要件 P-02 / P-03）。
 *
 * **月ごとの表は保存していない。** 起点とブロックから毎回計算される。
 * 起点を切り替えると計画を引き直したことになる。
 */
export function MonthlyTargetsView({ initial, initialMessage, loadMonthlyTargets }: Props) {
  const [targets, setTargets] = useState<MonthlyTargets | null>(initial);
  const [message, setMessage] = useState<string | null>(initialMessage);
  const [baseline, setBaseline] = useState<Baseline>("configured");
  const [pending, startTransition] = useTransition();

  function switchTo(next: Baseline) {
    setBaseline(next);
    setMessage(null);

    startTransition(async () => {
      const res = await loadMonthlyTargets(next);
      if (!res.ok) {
        // **黙って設定値に落とさない。** どちらの起点で見ているか
        // 分からないまま数字を読むと判断を誤る
        setTargets(null);
        setMessage(res.message);

        return;
      }
      setTargets(res.targets);
    });
  }

  return (
    <section className="rounded-2xl border border-line bg-surface p-4">
      <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
        <h2 className="text-sm font-medium">月次目標</h2>
        <div className="flex gap-2">
          <Toggle active={baseline === "configured"} onClick={() => switchTo("configured")}>
            設定値から
          </Toggle>
          <Toggle active={baseline === "measured"} onClick={() => switchTo("measured")}>
            実測から引き直す
          </Toggle>
        </div>
      </div>

      {targets && (
        <p className="tnum mb-3 text-xs text-muted">
          起点 {targets.baseline.month} ／{" "}
          {targets.baseline.source === "measured" ? "実測7日平均" : "設定値"} ／{" "}
          {targets.baseline.weightKg.toFixed(1)}kg ／ {targets.baseline.bodyfatPct.toFixed(1)}%
        </p>
      )}

      {message && (
        <p
          role="status"
          className="rounded-xl border border-warn/40 bg-warn/10 px-3 py-2 text-sm text-warn"
        >
          {message}
        </p>
      )}

      {pending && <p className="text-sm text-muted">計算中…</p>}

      {targets && targets.items.length > 0 && (
        /* 月が 36 行並ぶ。狭い画面では横に流す */
        <div className="-mx-4 overflow-x-auto px-4">
          <table className="tnum w-full min-w-[30rem] text-right text-sm">
            <thead>
              <tr className="text-[11px] text-muted">
                <th className="py-1 text-left font-normal">月</th>
                <th className="py-1 text-left font-normal">フェーズ</th>
                <th className="py-1 font-normal">体重</th>
                <th className="py-1 font-normal">BF%</th>
                <th className="py-1 font-normal">LBM</th>
                <th className="py-1 font-normal">FFMI</th>
              </tr>
            </thead>
            <tbody>
              {targets.items.map((t) => (
                <tr key={t.month} className="border-t border-line/60">
                  <td className="py-1.5 text-left">{t.month}</td>
                  <td className="max-w-32 truncate py-1.5 text-left text-muted">{t.phase}</td>
                  <td className="py-1.5">{t.weightKg.toFixed(1)}</td>
                  <td className="py-1.5">{t.bodyfatPct.toFixed(1)}</td>
                  <td className="py-1.5 text-muted">{t.lbmKg.toFixed(1)}</td>
                  <td className="py-1.5 text-muted">{t.ffmi.toFixed(1)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {targets?.note && <p className="mt-2 text-xs text-muted">{targets.note}</p>}
    </section>
  );
}

function Toggle({
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
      className={`pressable shrink-0 rounded-full border px-3 py-1.5 text-xs ${
        active
          ? "border-accent bg-accent font-semibold text-accent-ink"
          : "border-line bg-surface-2 text-muted"
      }`}
    >
      {children}
    </button>
  );
}
