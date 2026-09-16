"use server";

import { revalidatePath } from "next/cache";

import {
  ApiError,
  type CsvResource,
  type ImportResult,
  type Plan,
  type PlanBlock,
} from "@/lib/api/client";
import { serverApi } from "@/lib/api/server";

export type ExportResult = { ok: true; csv: string } | { ok: false; message: string };
export type PlanExport =
  { ok: true; plan: Plan; blocks: PlanBlock[] } | { ok: false; message: string };
export type PlanImport = { ok: boolean; message?: string };
export type ImportOutcome = { ok: true; result: ImportResult } | { ok: false; message: string };

/** CSV を書き出す（要件 I-02）。本文はそのままブラウザへ返す。 */
export async function exportCsv(
  resource: CsvResource,
  from: string,
  to: string,
): Promise<ExportResult> {
  try {
    const csv = await serverApi().exportCsv({
      resource,
      from: from === "" ? undefined : from,
      to: to === "" ? undefined : to,
    });

    return { ok: true, csv };
  } catch (e) {
    return { ok: false, message: reason(e) };
  }
}

/**
 * CSV を取り込む（要件 I-01）。
 *
 * **ファイルは FormData のまま渡す。** Server Action の引数に File を入れると
 * Next が直列化するが、そこで中身を読み直すと大きいファイルでメモリに二重に載る。
 */
export async function importCsv(form: FormData): Promise<ImportOutcome> {
  try {
    const result = await serverApi().importCsv(form);
    // 取り込んだデータは全画面に効く
    revalidatePath("/", "layout");

    return { ok: true, result };
  } catch (e) {
    return { ok: false, message: reason(e) };
  }
}

/**
 * 計画の設定を書き出す（要件 I-02 / ADR-0015）。
 *
 * **記録の CSV には設定が含まれない。** DB を消すと3年計画の設計値が消えるので、
 * こちらも保管の対象にする。API は既にあるものをそのまま使う。
 */
export async function exportPlan(): Promise<PlanExport> {
  try {
    const api = serverApi();
    const [plan, { items: blocks }] = await Promise.all([api.getPlan(), api.listPlanBlocks()]);

    return { ok: true, plan, blocks };
  } catch (e) {
    return { ok: false, message: reason(e) };
  }
}

/**
 * 計画の設定を読み込む。**まるごと置き換える。**
 *
 * ブロックを先に入れる。順序が逆だと、プランだけ新しくブロックが古い
 * 中途半端な状態で失敗しうる。
 */
export async function importPlan(plan: Plan, blocks: PlanBlock[]): Promise<PlanImport> {
  try {
    const api = serverApi();
    await api.putPlanBlocks(blocks);
    await api.putPlan({
      heightCm: plan.heightCm ?? null,
      startDate: plan.startDate ?? null,
      baselineWeightKg: plan.baselineWeightKg ?? null,
      baselineBodyfatPct: plan.baselineBodyfatPct ?? null,
      baselineMonth: plan.baselineMonth ?? null,
      phases: plan.phases,
      nutrition: plan.nutrition,
      volumeRanges: plan.volumeRanges,
    });
    revalidatePath("/", "layout");

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

  return e instanceof Error ? e.message : "処理できなかった";
}
