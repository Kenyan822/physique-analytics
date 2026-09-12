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

/** Date を JST の日付（YYYY-MM-DD）にする。 */
export function toJstDate(d: Date): string {
  const jst = new Date(d.getTime() + (JST_OFFSET_MIN + d.getTimezoneOffset()) * 60_000);

  const y = jst.getFullYear();
  const m = String(jst.getMonth() + 1).padStart(2, "0");
  const day = String(jst.getDate()).padStart(2, "0");

  return `${y}-${m}-${day}`;
}

/** YYYY-MM-DD を「9/13(土)」のような表示にする。 */
export function formatJstDate(date: string): string {
  const [y, m, d] = date.split("-").map(Number);
  if (!y || !m || !d) return date;

  // 曜日の計算だけに使う。表示は文字列から組み立てる
  const wd = ["日", "月", "火", "水", "木", "金", "土"][new Date(Date.UTC(y, m - 1, d)).getUTCDay()];

  return `${m}/${d}(${wd})`;
}
