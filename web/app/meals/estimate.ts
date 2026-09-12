import type { MealEstimate, MealSource } from "@/lib/api/client";

/**
 * 入力中の1件。**数値は文字列のまま持つ**（小数を打てるようにするため）。
 *
 * `source` を持たせているのは、**推定から来たのか手入力かを記録に残す**ため。
 * 推定値の比率が高い週は体重トレンドとの整合が取れない可能性があり、
 * 分析の確度を下げて扱う（docs/02-データモデル.md）。
 */
export type Draft = {
  name: string;
  qty: string;
  kcal: string;
  proteinG: string;
  fatG: string;
  carbG: string;
  source: MealSource;
};

export const EMPTY_DRAFT: Draft = {
  name: "",
  qty: "",
  kcal: "",
  proteinG: "",
  fatG: "",
  carbG: "",
  source: "manual",
};

/**
 * 推定結果を編集できる下書きにする（要件 N-06）。
 *
 * **そのまま保存しない。** 写真だけでは食器のサイズが分からないので、
 * 目視で直してから記録する前提になっている（openapi.yaml）。
 */
export function toDraft(estimate: MealEstimate): Draft {
  return {
    name: estimate.name,
    qty: estimate.qty ?? "",
    kcal: numText(estimate.kcal),
    proteinG: numText(estimate.proteinG),
    fatG: numText(estimate.fatG),
    carbG: numText(estimate.carbG),
    source: estimate.source,
  };
}

const CONFIDENCE: Record<string, string> = {
  low: "低",
  medium: "中",
  high: "高",
};

/** 確信度の表示。**知らない値は握りつぶさずそのまま出す。** */
export function confidenceLabel(confidence: string | null | undefined): string | null {
  if (!confidence) return null;

  return `確信度 ${CONFIDENCE[confidence] ?? confidence}`;
}

/** **推定できなかった値は空欄にする。** 0 で埋めると実績が過小になる */
function numText(v: number | null | undefined): string {
  return v == null ? "" : String(v);
}
