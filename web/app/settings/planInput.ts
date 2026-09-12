import type { NutritionSettings, Plan, PlanInput, PlanPhase, VolumeRange } from "@/lib/api/client";

/** 設定画面で編集している値。数値は入力中の文字列のまま持つ。 */
export type PlanFormValues = {
  heightCm: string;
  baselineWeightKg: string;
  baselineBodyfatPct: string;
  baselineMonth: string;
  phases: PlanPhase[];
  nutrition: NutritionSettings;
  volumeRanges: VolumeRange[];
};

/**
 * 保存する形に直す。
 *
 * **`PUT /v1/plan` はまるごと置き換わる。** 送らなかった項目は null になるので、
 * 画面で編集していない項目も必ず載せる（起点を消さない）。
 */
export function toPlanInput(plan: Plan, form: PlanFormValues): PlanInput {
  return {
    heightCm: toNumber(form.heightCm),
    // 開始日は画面に無い。読んだ値をそのまま返す
    startDate: plan.startDate ?? null,
    baselineWeightKg: toNumber(form.baselineWeightKg),
    baselineBodyfatPct: toNumber(form.baselineBodyfatPct),
    baselineMonth: form.baselineMonth.trim() === "" ? null : form.baselineMonth.trim(),
    phases: form.phases,
    nutrition: form.nutrition,
    volumeRanges: form.volumeRanges,
  };
}

/** YYYY-MM か。月次目標の起点月に使う。 */
export function isMonth(value: string): boolean {
  return /^\d{4}-(0[1-9]|1[0-2])$/.test(value);
}

/** 空欄は null。日本語入力のままだと全角数字になるので NFKC で正規化する。 */
function toNumber(text: string): number | null {
  const normalized = text.normalize("NFKC").trim();
  if (normalized === "") return null;

  const n = Number(normalized);

  return Number.isFinite(n) ? n : null;
}
