"use server";

import { revalidatePath } from "next/cache";

import { ApiError, type Template, type TemplateInput } from "@/lib/api/client";
import { serverApi } from "@/lib/api/server";

export type TemplateResult = { ok: true; template: Template } | { ok: false; message: string };

/** テンプレートを作る（要件 P-06）。 */
export async function createTemplate(input: TemplateInput): Promise<TemplateResult> {
  return run(() => serverApi().createTemplate(input));
}

/** テンプレートを更新する。項目は全入れ替え。 */
export async function updateTemplate(id: string, input: TemplateInput): Promise<TemplateResult> {
  return run(() => serverApi().updateTemplate(id, input));
}

/** テンプレートを削除する。 */
export async function deleteTemplate(id: string): Promise<{ ok: boolean; message?: string }> {
  try {
    await serverApi().deleteTemplate(id);
    revalidatePath("/settings");
    // 記録画面のテンプレート選択にも効く
    revalidatePath("/log");

    return { ok: true };
  } catch (e) {
    return { ok: false, message: reason(e) };
  }
}

async function run(fn: () => Promise<Template>): Promise<TemplateResult> {
  try {
    const template = await fn();
    revalidatePath("/settings");
    revalidatePath("/log");

    return { ok: true, template };
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
