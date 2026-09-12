"use server";

import { revalidatePath } from "next/cache";

import { ApiError, type BodyMeasurementInput, type DailyMetricsInput } from "@/lib/api/client";
import { serverApi } from "@/lib/api/server";

export type SaveResult = { ok: true } | { ok: false; message: string };

/**
 * 日次記録を保存する（要件 B-06 ほか）。
 *
 * **送った項目だけが変わる。** 朝に体重、夜に食事、という入力を想定しているので、
 * 画面で触っていない項目は送らない（API 側で null は「変更しない」）。
 */
export async function saveDaily(input: DailyMetricsInput): Promise<SaveResult> {
  return save(() => serverApi().putDailyMetrics(input));
}

/** 周囲長を保存する（要件 B-02）。 */
export async function saveMeasurement(input: BodyMeasurementInput): Promise<SaveResult> {
  return save(() => serverApi().putMeasurement(input));
}

async function save(fn: () => Promise<unknown>): Promise<SaveResult> {
  try {
    await fn();
    revalidatePath("/body");

    return { ok: true };
  } catch (e) {
    if (e instanceof ApiError) {
      // API が返した理由をそのまま見せる。何が悪いか分からないと直せない
      return { ok: false, message: e.problem?.detail ?? e.problem?.title ?? e.message };
    }

    return { ok: false, message: e instanceof Error ? e.message : "保存できなかった" };
  }
}
