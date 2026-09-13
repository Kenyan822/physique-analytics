import type { PlanBlock } from "@/lib/api/client";
import { parseInput } from "@/lib/number";

export type BlockError = { index: number; message: string };

/**
 * 保存する前に画面で気づけるようにする（要件 P-02）。
 *
 * **サーバ側の validatePlanBlock と同じ条件にしてある。** 往復してから
 * 怒られるより、その場で分かる方が速い。片方だけ変えると食い違うので、
 * 条件を変えるときは `api/internal/handler/plan_block.go` も直す。
 */
export function findBlockError(blocks: PlanBlock[]): BlockError | null {
  for (const [index, b] of blocks.entries()) {
    const message = messageFor(b);
    if (message !== "") return { index, message };
  }

  return null;
}

/** 計画全体の長さ。3年（36ヶ月）に対して足りているかを見る。 */
export function totalMonths(blocks: PlanBlock[]): number {
  return blocks.reduce((sum, b) => sum + b.months, 0);
}

function messageFor(b: PlanBlock): string {
  if (b.name.trim() === "") return "名前が空";
  if (b.months < 1 || b.months > 60) return "月数は 1〜60 にする";
  if (b.lbmDeltaKgPerMonth < -2 || b.lbmDeltaKgPerMonth > 2) {
    return "LBM の増減は ±2kg/月 以内にする";
  }
  if (b.bodyfatPctEnd <= 0 || b.bodyfatPctEnd >= 60) {
    return "終了時点の体脂肪率は 0 より大きく 60 未満にする";
  }

  return "";
}

/**
 * 符号付きの入力欄を読む。
 *
 * **`parseInput` は符号を読まない**（重量やレップに負は無いため）。
 * 減量ブロックの LBM 増減は負になるので、ここで符号を切り離して読み直す。
 * 読めなければ直前の値に戻す。
 */
export function readSigned(text: string, fallback: number): number {
  const normalized = text.normalize("NFKC").trim();
  if (!normalized.startsWith("-")) return parseInput(normalized, fallback);

  // 読めなければ NaN が返る。「-」だけ打っている途中もここに来る
  const body = parseInput(normalized.slice(1), NaN);

  return Number.isFinite(body) ? -body : fallback;
}
