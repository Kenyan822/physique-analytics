/**
 * 入力欄で使う数値の共通処理。
 *
 * 重量・レップ（記録画面）と体重・周囲長（体組成画面）で同じ挙動にするため、
 * 丸めと文字列の読み取りをここに寄せている。
 */

/**
 * 小数第2位で丸める。
 *
 * `v * 100` ではなく文字列で指数をずらしているのは、二進で丸めると
 * 1.005 が 1.00 になるため（1.005 * 100 は 100.49999… になる）。
 * 指数表記になる極端な値は桁シフトが NaN になるので、丸めずに返す。
 */
export function round2(v: number): number {
  const shifted = Number(`${v}e2`);

  return Number.isFinite(shifted) ? Number(`${Math.round(shifted)}e-2`) : v;
}

/**
 * 入力欄の文字列を数値にする。読めなければ `fallback`（＝直前の値）に戻す。
 *
 * NFKC で正規化しているのは、日本語入力のまま打つと全角数字になるため。
 * 弾くと「打てない」に見えるので、読めるものは読む。
 */
export function parseInput(text: string, fallback: number): number {
  const normalized = text.normalize("NFKC").trim();
  if (!/^\d*\.?\d*$/.test(normalized) || normalized === "" || normalized === ".") {
    return fallback;
  }

  const n = Number(normalized);

  return Number.isFinite(n) ? n : fallback;
}

/**
 * 前回値との差分を表示用の文字列にする（要件 B-03）。
 *
 * 差が無い・前回が無い場合は null。**測っただけで変化が見えないと、
 * 続ける意味が分からなくなる**ので、その場で差を出す。
 */
export function formatDiff(
  current: number | null,
  previous: number | null | undefined,
): string | null {
  if (current == null || previous == null) return null;

  const diff = round2(current - previous);
  if (diff === 0) return null;

  return diff > 0 ? `+${diff}` : `−${Math.abs(diff)}`;
}
