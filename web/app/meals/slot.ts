import type { MealSlot } from "@/lib/api/client";
import { jstHour } from "@/lib/jst";

/**
 * 時刻から区分を推測する。毎回選ばせると記録が面倒になる。
 *
 * **JST で判定する**（ADR-0013）。`Date#getHours()` は実行環境のタイムゾーンで
 * 評価されるので、UTC で動くサーバと JST のブラウザで別の答えになる。
 */
export function defaultSlot(now: Date = new Date()): MealSlot {
  const h = jstHour(now);
  if (h < 10) return "朝食";
  if (h < 15) return "昼食";
  if (h < 21) return "夕食";

  return "間食";
}
