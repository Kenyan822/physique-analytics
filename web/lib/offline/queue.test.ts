import { describe, expect, it, beforeEach } from "vitest";

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

describe("PendingQueue", () => {
  let q: PendingQueue;

  beforeEach(() => {
    q = new PendingQueue(new MemoryStore());
  });

  it("積んだものを順に返す", async () => {
    const a = pending({ setNo: 1 });
    const b = pending({ setNo: 2 });
    await q.push(a);
    await q.push(b);

    const all = await q.all();
    expect(all.map((x) => x.setNo)).toEqual([1, 2]);
  });

  // 同じ id での再送で二重に積まれると、復帰時に同じセットが2本入る
  it("同じ id は二重に積まない", async () => {
    const a = pending();
    await q.push(a);
    await q.push({ ...a, weightKg: 999 });

    const all = await q.all();
    expect(all).toHaveLength(1);
    // 後から来た内容で上書きする（同じ端末の訂正とみなす）
    expect(all[0].weightKg).toBe(999);
  });

  it("送れたものを取り除く", async () => {
    const a = pending({ setNo: 1 });
    const b = pending({ setNo: 2 });
    await q.push(a);
    await q.push(b);

    await q.remove([a.id]);

    const all = await q.all();
    expect(all.map((x) => x.id)).toEqual([b.id]);
  });

  it("存在しない id の削除は何もしない", async () => {
    await q.push(pending());
    await q.remove(["いない"]);

    expect(await q.all()).toHaveLength(1);
  });

  it("件数を返す", async () => {
    expect(await q.size()).toBe(0);
    await q.push(pending());
    expect(await q.size()).toBe(1);
  });

  it("空にできる", async () => {
    await q.push(pending());
    await q.clear();
    expect(await q.size()).toBe(0);
  });

  // 保存の形式が壊れていても、起動できなくなってはいけない
  it("壊れた保存内容は捨てて空から始める", async () => {
    const store = new MemoryStore();
    store.set("physique.pending", "{壊れている");
    const broken = new PendingQueue(store);

    expect(await broken.all()).toEqual([]);
  });

  it("配列でない保存内容も捨てる", async () => {
    const store = new MemoryStore();
    store.set("physique.pending", JSON.stringify({ not: "an array" }));
    const broken = new PendingQueue(store);

    expect(await broken.all()).toEqual([]);
  });
});
