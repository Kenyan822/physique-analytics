"use server";

import { revalidatePath } from "next/cache";

import { ApiError, type BloodTest, type BloodTestInput } from "@/lib/api/client";
import { serverApi } from "@/lib/api/server";

export type BloodTestResult = { ok: true; test: BloodTest } | { ok: false; message: string };

/** 血液検査を登録する（要件 B-08）。 */
export async function createBloodTest(input: BloodTestInput): Promise<BloodTestResult> {
  try {
    const test = await serverApi().createBloodTest(input);
    revalidatePath("/blood");

    return { ok: true, test };
  } catch (e) {
    return { ok: false, message: reason(e) };
  }
}

/** 血液検査を削除する。 */
export async function deleteBloodTest(id: string): Promise<{ ok: boolean; message?: string }> {
  try {
    await serverApi().deleteBloodTest(id);
    revalidatePath("/blood");

    return { ok: true };
  } catch (e) {
    return { ok: false, message: reason(e) };
  }
}

function reason(e: unknown): string {
  if (e instanceof ApiError) {
    const field = e.problem?.errors?.[0];
    if (field) return `${field.field}: ${field.message}`;
    // 409（同じ日の検査がある）は detail に直し方が入っている
    const detail = e.problem?.detail;

    return detail ? `${e.problem?.title ?? ""} ${detail}`.trim() : (e.problem?.title ?? e.message);
  }

  return e instanceof Error ? e.message : "保存できなかった";
}
