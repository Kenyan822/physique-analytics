import type { Plan, PlanBlock } from "@/lib/api/client";

/**
 * 設定のバックアップ形式（要件 I-02 / ADR-0015）。
 *
 * **記録（CSV）とは別ファイルにする。** 設定は入れ子の構造を持つので
 * CSV に収まらない。JSON にして1ファイルで往復させる。
 */
export type PlanBackup = {
  /** 形式の版。読み込み側が古いファイルを判別できるようにする */
  version: 1;
  /** 書き出した日時（UTC の ISO8601）。どの時点の設定かを残す */
  exportedAt: string;
  plan: Plan;
  blocks: PlanBlock[];
};

export function toBackup(plan: Plan, blocks: PlanBlock[], now: Date = new Date()): PlanBackup {
  return { version: 1, exportedAt: now.toISOString(), plan, blocks };
}

export type ParseResult = { ok: true; backup: PlanBackup } | { ok: false; message: string };

/**
 * 読み込んだ JSON を検証する。
 *
 * **形だけ見て中身の妥当性は見ない。** 値の検証はサーバ側が持っており
 * （フェーズの重なり、ブロックの月数など）、ここで二重に書くとずれる。
 */
export function parseBackup(text: string): ParseResult {
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    return { ok: false, message: "JSON として読めない" };
  }

  if (typeof raw !== "object" || raw === null) {
    return { ok: false, message: "JSON として読めない" };
  }

  const o = raw as Record<string, unknown>;
  if (o.version !== 1) {
    return { ok: false, message: `未知の形式（version: ${String(o.version)}）` };
  }
  if (typeof o.plan !== "object" || o.plan === null) {
    return { ok: false, message: "plan が無い" };
  }
  if (!Array.isArray(o.blocks)) {
    return { ok: false, message: "blocks が配列ではない" };
  }

  return { ok: true, backup: o as unknown as PlanBackup };
}

/** 書き出すファイル名。いつの設定かを名前に残す。 */
export function backupFileName(exportedAt: string): string {
  return `plan_${exportedAt.slice(0, 10)}.json`;
}
