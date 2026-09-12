"use client";

import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";

import { flushPending, type SendResult } from "./flush";
import { LocalStore, PendingQueue, type PendingSet } from "./queue";

/** 未送信が残っている間の再送間隔 */
const RETRY_INTERVAL_MS = 30_000;

type Options = {
  send: (item: PendingSet) => Promise<SendResult>;
};

/**
 * オンライン状態を購読する。
 *
 * **`useEffect` + `setState` ではなく `useSyncExternalStore` を使う。**
 * ブラウザが持つ外部状態なので、これが本来の API。
 * サーバ側では常に true にする（SSR では navigator が無い）。
 */
function subscribeOnline(callback: () => void): () => void {
  window.addEventListener("online", callback);
  window.addEventListener("offline", callback);

  return () => {
    window.removeEventListener("online", callback);
    window.removeEventListener("offline", callback);
  };
}

function useOnline(): boolean {
  return useSyncExternalStore(
    subscribeOnline,
    () => navigator.onLine,
    // SSR。サーバでは「オンライン」として描き、ハイドレート後に実際の値になる
    () => true,
  );
}

/**
 * オフライン時の記録キューを扱う（要件 T-07）。
 *
 * **記録は必ずキューに積んでから送る。** 「送ってみて失敗したら積む」に
 * すると、送信中にタブを閉じた記録が消える。
 */
export function useOfflineQueue({ send }: Options) {
  const queue = useRef<PendingQueue | null>(null);
  const [pendingCount, setPendingCount] = useState(0);
  const online = useOnline();

  queue.current ??= new PendingQueue(new LocalStore());

  const refresh = useCallback(async () => {
    setPendingCount(await queue.current!.size());
  }, []);

  const flush = useCallback(async () => {
    const res = await flushPending(queue.current!, {
      send,
      online: () => navigator.onLine,
    });
    await refresh();

    return res;
  }, [send, refresh]);

  // オンラインになったら流す。起動直後も一度流す
  // （前回の終了時に送れなかった分が残っている）
  useEffect(() => {
    if (!online) return;

    void flush();
  }, [online, flush]);

  /**
   * 残っている間は定期的に再送する。
   *
   * **`online` イベントだけでは足りない。** 電波はあるがサーバ側が落ちている、
   * ジムの Wi-Fi がキャプティブポータルで塞がれている、といった場合は
   * `navigator.onLine` が true のままなので、再送の契機が無くなる。
   */
  useEffect(() => {
    if (!online || pendingCount === 0) return;

    const id = setInterval(() => void flush(), RETRY_INTERVAL_MS);

    return () => clearInterval(id);
  }, [online, pendingCount, flush]);

  /** 記録を積んで、可能ならすぐ送る。戻り値は「今サーバに届いたか」。 */
  const record = useCallback(
    async (item: PendingSet): Promise<{ synced: boolean; message?: string }> => {
      await queue.current!.push(item);
      await refresh();

      const res = await flush();
      if (res.skipped) return { synced: false };
      if (res.failed > 0) {
        return { synced: false, message: "送信できなかった。オンライン復帰時に再送する" };
      }

      return { synced: true };
    },
    [flush, refresh],
  );

  return { record, flush, pendingCount, online };
}
