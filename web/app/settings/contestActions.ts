"use server";

import { revalidatePath } from "next/cache";

import { ApiError, type Contest, type ContestInput } from "@/lib/api/client";
import { serverApi } from "@/lib/api/server";

export type ContestResult = { ok: true; contest: Contest } | { ok: false; message: string };

/** 大会を登録する（要件 P-04）。 */
export async function createContest(input: ContestInput): Promise<ContestResult> {
  try {
    const contest = await serverApi().createContest(input);
    revalidatePath("/settings");
    // 次の大会は日別目標のカウントダウンに出る（要件 A-10）
    revalidatePath("/meals");

    return { ok: true, contest };
  } catch (e) {
    return { ok: false, message: reason(e) };
  }
}

/** 大会を削除する。 */
export async function deleteContest(id: string): Promise<{ ok: boolean; message?: string }> {
  try {
    await serverApi().deleteContest(id);
    revalidatePath("/settings");
    revalidatePath("/meals");

    return { ok: true };
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

  return e instanceof Error ? e.message : "保存できなかった";
}
