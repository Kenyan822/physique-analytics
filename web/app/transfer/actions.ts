"use server";

import { revalidatePath } from "next/cache";

import { ApiError, type CsvResource, type ImportResult } from "@/lib/api/client";
import { serverApi } from "@/lib/api/server";

export type ExportResult = { ok: true; csv: string } | { ok: false; message: string };
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

function reason(e: unknown): string {
  if (e instanceof ApiError) {
    const field = e.problem?.errors?.[0];

    return field
      ? `${field.field}: ${field.message}`
      : (e.problem?.detail ?? e.problem?.title ?? e.message);
  }

  return e instanceof Error ? e.message : "処理できなかった";
}
