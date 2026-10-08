import type { ManualTargetEntry, ManualTargets } from "@/lib/api/client";

/** 入力欄の中身。**数値に直しながら持たない**（「62.」で丸められて小数が打てない） */
export type TargetDraft = {
  proteinG: string;
  fatG: string;
  carbG: string;
};

/**
 * PFC から kcal を出す（Atwater 係数 4/9/4）。
 *
 * **サーバの `analytics.KcalFromMacros` と同じ式にする。** 保存すると
 * サーバが計算し直すので、食い違うと画面の値が保存後に変わって見える。
 */
export function kcalFromMacros(proteinG: number, fatG: number, carbG: number): number {
  return Math.round(proteinG * 4 + fatG * 9 + carbG * 4);
}

/** 上限は openapi.yaml に合わせる */
const MAX = { proteinG: 1000, fatG: 1000, carbG: 2000 } as const;

/**
 * 入力を目標にする。**読めなければ null。**
 *
 * PFC は3つで1組。1つ欠けた目標は意味を成さないので、
 * 部分的に埋まった状態では保存させない（iOS と同じ規則）。
 */
export function toManualTargets(draft: TargetDraft): ManualTargets | null {
  const proteinG = num(draft.proteinG, MAX.proteinG);
  const fatG = num(draft.fatG, MAX.fatG);
  const carbG = num(draft.carbG, MAX.carbG);

  if (proteinG === null || fatG === null || carbG === null) return null;

  return { proteinG, fatG, carbG };
}

function num(v: string, max: number): number | null {
  const t = v.trim();
  if (t === "" || !/^\d*\.?\d*$/.test(t)) return null;

  const n = Number(t);

  return Number.isFinite(n) && n >= 0 && n <= max ? n : null;
}

/** 履歴の1行が、いまどういう状態か */
export type EntryStatus = "current" | "scheduled" | "past";

/**
 * 履歴の各行の状態を返す（キーは id）。
 *
 * **開始日が今日以前で最新の行が適用中。** サーバの
 * `starts_on <= 日付 order by starts_on desc limit 1` と同じ規則。
 * `entries` は開始日の新しい順（サーバが並べて返す）。
 */
export function entryStatuses(
  entries: ManualTargetEntry[],
  today: string,
): Record<string, EntryStatus> {
  const out: Record<string, EntryStatus> = {};
  let currentFound = false;

  for (const e of entries) {
    if (e.startsOn > today) {
      out[e.id] = "scheduled";
    } else if (!currentFound) {
      out[e.id] = "current";
      currentFound = true;
    } else {
      out[e.id] = "past";
    }
  }

  return out;
}
