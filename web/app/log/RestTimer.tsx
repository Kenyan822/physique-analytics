"use client";

import { useEffect, useRef, useState } from "react";

import { formatRemaining } from "./rest";

type Props = {
  /** 休憩秒数。0 なら表示しない */
  seconds: number;
  /** セットを記録した時刻（ms）。これが変わるとタイマーが再開する */
  startedAt: number | null;
};

const RADIUS = 26;
const CIRCUMFERENCE = 2 * Math.PI * RADIUS;

/**
 * インターバルタイマー（要件 T-05）。
 *
 * **`setInterval` でカウントダウンしない。** 画面を消している間は
 * タイマーが止まるので、戻ってきたときに残り時間がずれる。
 * 開始時刻を持っておいて、描画のたびに「今との差」を計算する。
 *
 * リングにしているのは、数字を読まなくても「あとどれくらいか」が
 * 目の端で分かるため。セット間はスマホをちゃんと見ていない。
 */
export function RestTimer({ seconds, startedAt }: Props) {
  const [now, setNow] = useState(() => Date.now());
  const notified = useRef<number | null>(null);

  useEffect(() => {
    if (!startedAt || seconds <= 0) return;

    // 期限に達したら止める。次のセットを記録するまで restStartedAt は
    // 残るので、止めないと 250ms ごとの再描画が延々と走り続ける
    const id = setInterval(() => {
      const t = Date.now();
      setNow(t);
      if (t - startedAt >= seconds * 1000) clearInterval(id);
    }, 250);

    return () => clearInterval(id);
  }, [startedAt, seconds]);

  const remaining = startedAt ? seconds - (now - startedAt) / 1000 : 0;
  const done = startedAt != null && remaining <= 0;
  const ratio = startedAt ? Math.min(1, Math.max(0, remaining / seconds)) : 0;

  useEffect(() => {
    if (!done || !startedAt || notified.current === startedAt) return;
    notified.current = startedAt;

    // 通知は許可されていれば出す。求めるのはユーザー操作の後だけにする
    if (typeof Notification !== "undefined" && Notification.permission === "granted") {
      new Notification("インターバル終了", { body: "次のセットへ" });
    }
  }, [done, startedAt]);

  if (!startedAt || seconds <= 0) return null;

  return (
    <div
      role="timer"
      aria-live="off"
      className={`flex items-center gap-3 rounded-2xl border px-4 py-3 ${
        done ? "border-accent bg-accent/10" : "border-line bg-surface"
      }`}
    >
      <svg width="64" height="64" viewBox="0 0 64 64" aria-hidden className="shrink-0 -rotate-90">
        <circle
          cx="32"
          cy="32"
          r={RADIUS}
          fill="none"
          stroke="currentColor"
          strokeWidth="5"
          className="text-line"
        />
        <circle
          cx="32"
          cy="32"
          r={RADIUS}
          fill="none"
          stroke="currentColor"
          strokeWidth="5"
          strokeLinecap="round"
          strokeDasharray={CIRCUMFERENCE}
          strokeDashoffset={CIRCUMFERENCE * (1 - ratio)}
          className={done ? "text-accent" : "text-accent"}
        />
      </svg>

      <div className="min-w-0">
        <div className="text-xs text-muted">{done ? "インターバル終了" : "インターバル"}</div>
        <div className="tnum text-3xl font-semibold leading-tight">
          {formatRemaining(remaining)}
        </div>
      </div>
    </div>
  );
}
