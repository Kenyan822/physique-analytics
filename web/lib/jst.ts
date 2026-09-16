/**
 * 日付は JST 固定（ADR-0013）。
 *
 * **`new Date().toISOString().slice(0, 10)` を使わない。** UTC に変換されるので、
 * 日本時間の 0:00〜9:00 に記録すると前日の日付になる。
 */
const JST_OFFSET_MIN = 9 * 60;

/** JST における「今日」を YYYY-MM-DD で返す。 */
export function todayJst(now: Date = new Date()): string {
  return toJstDate(now);
}

/**
 * JST における「時」を 0〜23 で返す。
 *
 * **`Date#getHours()` を使わない。** 実行環境のタイムゾーンで評価されるので、
 * UTC で動くサーバと JST のブラウザで別の答えになる（#175）。
 */
export function jstHour(now: Date = new Date()): number {
  return toJst(now).getUTCHours();
}

/** Date を JST の日付（YYYY-MM-DD）にする。 */
export function toJstDate(d: Date): string {
  const jst = toJst(d);

  const y = jst.getUTCFullYear();
  const m = String(jst.getUTCMonth() + 1).padStart(2, "0");
  const day = String(jst.getUTCDate()).padStart(2, "0");

  return `${y}-${m}-${day}`;
}

/**
 * JST の壁時計を UTC のフィールドに載せ替えた Date を返す。
 *
 * **返り値を時刻として使わない。** `getUTC*` で JST の年月日時を読むための器で、
 * 実際の瞬間としては9時間ずれている。
 */
function toJst(d: Date): Date {
  return new Date(d.getTime() + JST_OFFSET_MIN * 60_000);
}

/** YYYY-MM-DD を「9/13(土)」のような表示にする。 */
export function formatJstDate(date: string): string {
  const [y, m, d] = date.split("-").map(Number);
  if (!y || !m || !d) return date;

  // 曜日の計算だけに使う。表示は文字列から組み立てる
  const wd = ["日", "月", "火", "水", "木", "金", "土"][new Date(Date.UTC(y, m - 1, d)).getUTCDay()];

  return `${m}/${d}(${wd})`;
}
