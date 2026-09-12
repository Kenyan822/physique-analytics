/**
 * オフライン時の記録キュー（要件 T-07）。
 *
 * **ジムは電波が悪い。** 送信に失敗した記録をここに溜めて、
 * 復帰時にまとめて送る（API 側の `/v1/sync` push）。
 *
 * 溜めるのはセットだけ。セッションは日付から一意に決まるので、
 * 送信側で「その日のセッションを取るか作る」を済ませられる。
 */

const STORAGE_KEY = "physique.pending";

export type PendingSet = {
  /** クライアントが生成する UUID。再送の冪等性に使う（ADR-0014） */
  id: string;
  /** JST の日付（ADR-0013） */
  date: string;
  exerciseId: string;
  setNo: number;
  weightKg: number;
  reps: number;
  rir: number | null;
  /** 積んだ時刻。順序の確認と、古すぎるものの判別に使う */
  queuedAt: number;
};

/** 保存先。localStorage とテスト用のメモリを差し替えられるようにする。 */
export type Store = {
  get(key: string): string | null;
  set(key: string, value: string): void;
  remove(key: string): void;
};

export class MemoryStore implements Store {
  private data = new Map<string, string>();

  get(key: string) {
    return this.data.get(key) ?? null;
  }

  set(key: string, value: string) {
    this.data.set(key, value);
  }

  remove(key: string) {
    this.data.delete(key);
  }
}

/**
 * localStorage を使う保存先。
 *
 * **Safari のプライベートモードでは書き込みが例外になる。** 記録できないより
 * 「キューが効かない」方がましなので、失敗しても落とさない。
 */
export class LocalStore implements Store {
  get(key: string) {
    try {
      return localStorage.getItem(key);
    } catch {
      return null;
    }
  }

  set(key: string, value: string) {
    try {
      localStorage.setItem(key, value);
    } catch {
      // 容量超過やプライベートモード。握りつぶすが、呼び出し側は
      // size() が増えないことで気づける
    }
  }

  remove(key: string) {
    try {
      localStorage.removeItem(key);
    } catch {
      // 同上
    }
  }
}

export class PendingQueue {
  constructor(private store: Store) {}

  /** 積む。同じ id は上書きする（同じ端末からの訂正とみなす）。 */
  async push(item: PendingSet): Promise<void> {
    const items = this.read();
    const i = items.findIndex((x) => x.id === item.id);

    if (i >= 0) {
      items[i] = item;
    } else {
      items.push(item);
    }

    this.write(items);
  }

  /** 積んである全件を、積んだ順に返す。 */
  async all(): Promise<PendingSet[]> {
    return this.read();
  }

  /** 送れたものを取り除く。 */
  async remove(ids: string[]): Promise<void> {
    const drop = new Set(ids);
    this.write(this.read().filter((x) => !drop.has(x.id)));
  }

  async size(): Promise<number> {
    return this.read().length;
  }

  async clear(): Promise<void> {
    this.store.remove(STORAGE_KEY);
  }

  /**
   * 保存内容を読む。
   *
   * **壊れていたら捨てて空から始める。** 形式が変わったときや書き込みが
   * 途中で切れたときに、アプリが起動できなくなる方が困る。
   */
  private read(): PendingSet[] {
    const raw = this.store.get(STORAGE_KEY);
    if (!raw) return [];

    try {
      const parsed: unknown = JSON.parse(raw);
      if (!Array.isArray(parsed)) return [];

      return parsed.filter(isPendingSet);
    } catch {
      return [];
    }
  }

  private write(items: PendingSet[]): void {
    this.store.set(STORAGE_KEY, JSON.stringify(items));
  }
}

function isPendingSet(v: unknown): v is PendingSet {
  if (typeof v !== "object" || v === null) return false;
  const o = v as Record<string, unknown>;

  return (
    typeof o.id === "string" &&
    typeof o.date === "string" &&
    typeof o.exerciseId === "string" &&
    typeof o.setNo === "number" &&
    typeof o.weightKg === "number" &&
    typeof o.reps === "number" &&
    (o.rir === null || typeof o.rir === "number")
  );
}
