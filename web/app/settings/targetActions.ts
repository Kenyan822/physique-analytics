"use server";

import { revalidatePath } from "next/cache";

import { ApiError, type ManualTargets } from "@/lib/api/client";
import { serverApi } from "@/lib/api/server";

export type TargetResult = { ok: true } | { ok: false; message: string };

/**
 * 手で決めた摂取目標を保存する（要件 N-05）。
 *
 * **計画（`PUT /v1/plan`）とは別のエンドポイント。** あちらはまるごと
 * 置き換える作りで、混ぜると基準値が消える（#127 で踏んだ）。
 */
export async function saveManualTargets(input: ManualTargets): Promise<TargetResult> {
  try {
    await serverApi().putManualTargets(input);
    // 目標が変わると食事画面の残量も変わる
    revalidatePath("/settings");
    revalidatePath("/meals");

    return { ok: true };
  } catch (e) {
    return { ok: false, message: reason(e, "保存できなかった") };
  }
}

/** 手動の目標を消す。自動計算（A-02）に戻る。 */
export async function clearManualTargets(): Promise<TargetResult> {
  try {
    await serverApi().deleteManualTargets();
    revalidatePath("/settings");
    revalidatePath("/meals");

    return { ok: true };
  } catch (e) {
    return { ok: false, message: reason(e, "消せなかった") };
  }
}

function reason(e: unknown, fallback: string): string {
  if (e instanceof ApiError) {
    const field = e.problem?.errors?.[0];
    const detail = field ? `${field.field}: ${field.message}` : e.problem?.detail;

    return detail ?? e.problem?.title ?? e.message;
  }

  return e instanceof Error ? e.message : fallback;
}
