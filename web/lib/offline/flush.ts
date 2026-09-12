import type { PendingQueue, PendingSet } from "./queue";

/**
 * 溜まった記録をサーバへ送る（要件 T-07）。
 *
 * **1本ずつ順に送る。** セット番号の順序が崩れると、サーバ側の
 * 一意制約（session_id, exercise_id, set_no）に別の順序で当たる。
 */
export type SendResult = { ok: true } | { ok: false; message: string };

export type FlushDeps = {
  send: (item: PendingSet) => Promise<SendResult>;
  online: () => boolean;
};

export type FlushResult = {
  sent: number;
  failed: number;
  /** オフラインで何もしなかったか */
  skipped: boolean;
};

/**
 * サーバが「既にある」と答えたときは送信済みとみなす。
 *
 * 送信は届いたが応答が失われた場合にこうなる。**失敗として扱うと
 * 永久に送り直し続ける**ので、成功としてキューから外す。
 */
function isAlreadyRecorded(message: string): boolean {
  return message.includes("既にある") || message.includes("既に存在する");
}

export async function flushPending(queue: PendingQueue, deps: FlushDeps): Promise<FlushResult> {
  if (!deps.online()) {
    return { sent: 0, failed: 0, skipped: true };
  }

  const items = await queue.all();
  if (items.length === 0) {
    return { sent: 0, failed: 0, skipped: false };
  }

  const done: string[] = [];
  let failed = 0;

  for (const item of items) {
    try {
      const res = await deps.send(item);
      if (res.ok || isAlreadyRecorded(res.message)) {
        done.push(item.id);
        continue;
      }
      failed++;
    } catch {
      // ネットワークが切れた等。**キューから外さない**
      failed++;
    }
  }

  await queue.remove(done);

  return { sent: done.length, failed, skipped: false };
}
