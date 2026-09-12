"use client";

import { useEffect, useRef, useState } from "react";

import { formatRemaining } from "./rest";

type Props = {
  /** 休憩秒数。0 なら表示しない */
  seconds: number;
  /** セットを記録した時刻（ms）。これが変わるとタイマーが再開する */
  startedAt: number | null;
};

/**
 * インターバルタイマー（要件 T-05）。
 *
 * **`setInterval` でカウントダウンしない。** 画面を消している間 iOS は
 * タイマーを止めるので、戻ってきたときに残り時間がずれる。
 * 開始時刻を持っておいて、描画のたびに「今との差」を計算する。
 */
export function RestTimer({ seconds, startedAt }: Props) {
  const [now, setNow] = useState(() => Date.now());
  const notified = useRef<number | null>(null);

  useEffect(() => {
    if (!startedAt || seconds <= 0) return;

    const id = setInterval(() => setNow(Date.now()), 500);

    return () => clearInterval(id);
  }, [startedAt, seconds]);

  const remaining = startedAt ? seconds - (now - startedAt) / 1000 : 0;
  const done = startedAt != null && remaining <= 0;

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
      className={`rounded-lg p-3 text-center tabular-nums ${
        done
          ? "bg-green-100 text-green-900 dark:bg-green-950 dark:text-green-200"
          : "bg-gray-100 dark:bg-gray-900"
      }`}
    >
      <span className="text-sm text-gray-600 dark:text-gray-400">
        {done ? "インターバル終了" : "インターバル"}
      </span>
      <div className="text-3xl font-semibold">{formatRemaining(remaining)}</div>
    </div>
  );
}
