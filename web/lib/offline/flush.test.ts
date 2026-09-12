import { describe, expect, it, vi } from "vitest";

import { flushPending, type FlushDeps } from "./flush";
import { MemoryStore, PendingQueue, type PendingSet } from "./queue";

function pending(overrides: Partial<PendingSet> = {}): PendingSet {
  return {
    id: crypto.randomUUID(),
    date: "2026-09-13",
    exerciseId: "11111111-1111-1111-1111-111111111111",
    setNo: 1,
    weightKg: 80,
    reps: 5,
    rir: 2,
    queuedAt: Date.now(),
    ...overrides,
  };
}

function deps(overrides: Partial<FlushDeps> = {}): FlushDeps {
  return {
    send: vi.fn().mockResolvedValue({ ok: true }),
    online: () => true,
    ...overrides,
  };
}

describe("flushPending", () => {
  it("溜まったものを送って取り除く", async () => {
    const q = new PendingQueue(new MemoryStore());
    await q.push(pending({ setNo: 1 }));
    await q.push(pending({ setNo: 2 }));
    const d = deps();

    const res = await flushPending(q, d);

    expect(res).toEqual({ sent: 2, failed: 0, skipped: false });
    expect(d.send).toHaveBeenCalledTimes(2);
    expect(await q.size()).toBe(0);
  });

  it("オフラインなら何もしない", async () => {
    const q = new PendingQueue(new MemoryStore());
    await q.push(pending());
    const d = deps({ online: () => false });

    const res = await flushPending(q, d);

    expect(res).toEqual({ sent: 0, failed: 0, skipped: true });
    expect(d.send).not.toHaveBeenCalled();
    // 送っていないので消さない
    expect(await q.size()).toBe(1);
  });

  it("空なら送らない", async () => {
    const q = new PendingQueue(new MemoryStore());
    const d = deps();

    expect(await flushPending(q, d)).toEqual({ sent: 0, failed: 0, skipped: false });
    expect(d.send).not.toHaveBeenCalled();
  });

  // 送信に失敗したものはキューに残す。消すと記録が消える
  it("失敗したものは残す", async () => {
    const q = new PendingQueue(new MemoryStore());
    const a = pending({ setNo: 1 });
    const b = pending({ setNo: 2 });
    await q.push(a);
    await q.push(b);

    const send = vi
      .fn()
      .mockResolvedValueOnce({ ok: true })
      .mockResolvedValueOnce({ ok: false, message: "落ちた" });

    const res = await flushPending(q, deps({ send }));

    expect(res).toEqual({ sent: 1, failed: 1, skipped: false });
    const left = await q.all();
    expect(left.map((x) => x.id)).toEqual([b.id]);
  });

  // サーバが「既にある」と言うなら、送信は成功していて応答だけ失われている
  it("重複エラーは送信済みとして取り除く", async () => {
    const q = new PendingQueue(new MemoryStore());
    await q.push(pending());

    const send = vi.fn().mockResolvedValue({ ok: false, message: "セット番号 1 は既にある: 既に存在する" });

    const res = await flushPending(q, deps({ send }));

    expect(res.sent).toBe(1);
    expect(res.failed).toBe(0);
    expect(await q.size()).toBe(0);
  });

  it("send が例外を投げても落ちない", async () => {
    const q = new PendingQueue(new MemoryStore());
    await q.push(pending());

    const send = vi.fn().mockRejectedValue(new Error("ネットワーク"));

    const res = await flushPending(q, deps({ send }));

    expect(res).toEqual({ sent: 0, failed: 1, skipped: false });
    expect(await q.size()).toBe(1);
  });

  // 順序が崩れるとセット番号が入れ替わる
  it("積んだ順に送る", async () => {
    const q = new PendingQueue(new MemoryStore());
    for (const n of [1, 2, 3]) await q.push(pending({ setNo: n }));

    const order: number[] = [];
    const send = vi.fn().mockImplementation(async (s: PendingSet) => {
      order.push(s.setNo);
      return { ok: true };
    });

    await flushPending(q, deps({ send }));

    expect(order).toEqual([1, 2, 3]);
  });
});
