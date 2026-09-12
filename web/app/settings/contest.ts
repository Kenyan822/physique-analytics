import type { Contest } from "@/lib/api/client";

/**
 * その月の末日を YYYY-MM-DD で返す。
 *
 * **日程が決まる前の大会はこれを入れる**（openapi.yaml）。月内では
 * 最終日が一番遠いので、必要な減量ペース（要件 A-10）を過小評価しない。
 *
 * JST 固定なので UTC で計算してよい（ADR-0013）。
 */
export function endOfMonth(date: string): string {
  const [y, m] = date.split("-").map(Number);

  // 月は 1 始まり。翌月の 0 日目 ＝ その月の末日
  return new Date(Date.UTC(y, m, 0)).toISOString().slice(0, 10);
}

/** 開催日の昇順に並べる。近い大会から見る。 */
export function sortContests(contests: Contest[]): Contest[] {
  return [...contests].sort((a, b) => a.heldOn.localeCompare(b.heldOn));
}
