/**
 * インターバルタイマー（要件 T-05）。
 *
 * 種目ごとに既定値を持つ。**多関節は回復に時間が要る** ——
 * ベンチやスクワットで 90 秒しか休まないと次セットのレップが落ち、
 * 有効セット数が稼げない。
 */
const COMPOUND_REST_SEC = 180;
const ISOLATION_REST_SEC = 90;

export type RestTarget = {
  defaultRestSec?: number | null;
  isCompound?: boolean;
};

/** 次のセットまでの休憩秒数を決める。 */
export function nextRestSeconds({ defaultRestSec, isCompound }: RestTarget): number {
  // 0 は「休憩なし」という意思表示。未設定（null / undefined）と区別する
  if (defaultRestSec != null) {
    return Math.max(0, Math.round(defaultRestSec));
  }

  return isCompound ? COMPOUND_REST_SEC : ISOLATION_REST_SEC;
}

/** 残り秒数を 分:秒 にする。 */
export function formatRemaining(sec: number): string {
  const s = Math.max(0, Math.round(sec));
  const m = Math.floor(s / 60);

  return `${m}:${String(s % 60).padStart(2, "0")}`;
}
