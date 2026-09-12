import type { CsvResource } from "@/lib/api/client";

export const RESOURCES: CsvResource[] = ["daily", "workouts", "measures"];

const LABELS: Record<CsvResource, string> = {
  daily: "日次",
  workouts: "トレーニング",
  measures: "周囲長",
};

/**
 * 期待する列。**`api/internal/csvio/csvio.go` の header と同じ並び。**
 * 手で書いた CSV を直すとき、どの列が要るかをその場で見せるために持つ。
 * 片方だけ変えると取り込みが落ちるので、変えるときは両方直す。
 */
const COLUMNS: Record<CsvResource, string[]> = {
  daily: [
    "date",
    "weight_kg",
    "bodyfat_pct",
    "kcal",
    "protein_g",
    "fat_g",
    "carb_g",
    "sleep_h",
    "steps",
    "fatigue",
    "hrv_ms",
    "resting_hr",
    "deep_sleep_min",
    "note",
  ],
  workouts: ["date", "exercise", "set_no", "weight_kg", "reps", "rir"],
  measures: [
    "date",
    "neck_cm",
    "shoulder_cm",
    "chest_cm",
    "waist_navel_cm",
    "hip_cm",
    "arm_r_cm",
    "thigh_r_cm",
    "calf_r_cm",
  ],
};

export function resourceLabel(resource: CsvResource): string {
  return LABELS[resource];
}

export function columnsOf(resource: CsvResource): string[] {
  return COLUMNS[resource];
}

/**
 * 書き出すファイル名。**期間を名前に入れる。**
 * 「いつの分か分からない CSV」が手元に溜まると、バックアップとして使えない。
 */
export function exportFileName(resource: CsvResource, from: string, to: string): string {
  if (from === "" && to === "") return `${resource}_all.csv`;

  return `${resource}_${from || "all"}_${to || "all"}.csv`;
}
