"use server";

import { revalidatePath } from "next/cache";

import { ApiError, type MonthlyTargets, type PlanBlock } from "@/lib/api/client";
import { serverApi } from "@/lib/api/server";

export type SaveBlocksResult = { ok: true; items: PlanBlock[] } | { ok: false; message: string };

/** 起点の切り替え結果（要件 P-03）。422 は「出せない理由」なので message で返す。 */
export type TargetsResult = { ok: true; targets: MonthlyTargets } | { ok: false; message: string };

/** 計画のブロックをまるごと置き換える（要件 P-02）。 */
export async function saveBlocks(items: PlanBlock[]): Promise<SaveBlocksResult> {
  try {
    const res = await serverApi().putPlanBlocks(items);
    revalidatePath("/plan");

    return { ok: true, items: res.items };
  } catch (e) {
    return { ok: false, message: reason(e) };
  }
}

/**
 * 月次目標を引き直す（要件 P-03）。
 *
 * **実測が無いときに設定値へ黙って落ちない。** サーバが理由を返すので
 * そのまま画面に出す。どちらの起点で見ているか分からないまま数字を読むと
 * 判断を誤る。
 */
export async function loadMonthlyTargets(
  baseline: "configured" | "measured",
): Promise<TargetsResult> {
  try {
    return { ok: true, targets: await serverApi().monthlyTargets(baseline) };
  } catch (e) {
    return { ok: false, message: reason(e) };
  }
}

function reason(e: unknown): string {
  if (e instanceof ApiError) {
    const field = e.problem?.errors?.[0];

    return field
      ? `${field.field}: ${field.message}`
      : (e.problem?.detail ?? e.problem?.title ?? e.message);
  }

  return e instanceof Error ? e.message : "読めなかった";
}
