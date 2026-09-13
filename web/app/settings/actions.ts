"use server";

import { revalidatePath } from "next/cache";

import { ApiError, type Plan, type PlanInput } from "@/lib/api/client";
import { serverApi } from "@/lib/api/server";

export type SavePlanResult = { ok: true; plan: Plan } | { ok: false; message: string };

/** 計画の設定を保存する（要件 P-01 / P-05）。まるごと置き換わる。 */
export async function savePlan(input: PlanInput): Promise<SavePlanResult> {
  try {
    const plan = await serverApi().putPlan(input);
    // 目標が変わると食事画面の残量も変わる
    revalidatePath("/settings");
    revalidatePath("/meals");

    return { ok: true, plan };
  } catch (e) {
    if (e instanceof ApiError) {
      // 何が悪いかを返す。フィールド名が付いていれば添える
      const field = e.problem?.errors?.[0];
      const detail = field ? `${field.field}: ${field.message}` : e.problem?.detail;

      return { ok: false, message: detail ?? e.problem?.title ?? e.message };
    }

    return { ok: false, message: e instanceof Error ? e.message : "保存できなかった" };
  }
}
