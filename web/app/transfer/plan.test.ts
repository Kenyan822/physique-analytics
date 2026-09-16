import { describe, expect, it } from "vitest";

import type { Plan, PlanBlock } from "@/lib/api/client";

import { backupFileName, parseBackup, toBackup } from "./plan";

const plan: Plan = {
  heightCm: 175,
  baselineWeightKg: 75,
  baselineBodyfatPct: 20,
  baselineMonth: "2026-09",
  phases: [],
  nutrition: {
    cut: { proteinGPerKg: 2.2, fatGPerKg: 0.85 },
    deepCut: { proteinGPerKg: 2.4, fatGPerKg: 0.7 },
    bulk: { proteinGPerKg: 2, fatGPerKg: 1 },
    deepCutBfThreshold: 12,
    carbMinG: 100,
  },
  volumeRanges: [],
};

const blocks: PlanBlock[] = [
  { name: "Y1 増量", months: 8, lbmDeltaKgPerMonth: 0.3, bodyfatPctEnd: 16 },
];

describe("toBackup / parseBackup", () => {
  it("書き出したものをそのまま読み戻せる", () => {
    const json = JSON.stringify(toBackup(plan, blocks, new Date("2026-09-14T01:23:45Z")));
    const got = parseBackup(json);

    expect(got.ok).toBe(true);
    if (!got.ok) return;
    expect(got.backup.plan.heightCm).toBe(175);
    expect(got.backup.blocks).toHaveLength(1);
    expect(got.backup.exportedAt).toBe("2026-09-14T01:23:45.000Z");
  });

  it("起点が落ちない", () => {
    // ここが消えると月次目標を引き直せない（#126 と同じ穴）
    const got = parseBackup(JSON.stringify(toBackup(plan, blocks)));

    expect(got.ok).toBe(true);
    if (!got.ok) return;
    expect(got.backup.plan.baselineWeightKg).toBe(75);
    expect(got.backup.plan.baselineMonth).toBe("2026-09");
  });
});

describe("parseBackup が弾くもの", () => {
  it.each([
    ["JSON ではない", "これは JSON ではない"],
    ["version が違う", '{"version":2,"plan":{},"blocks":[]}'],
    ["version が無い", '{"plan":{},"blocks":[]}'],
    ["plan が無い", '{"version":1,"blocks":[]}'],
    ["blocks が配列ではない", '{"version":1,"plan":{},"blocks":{}}'],
    ["配列を渡した", "[]"],
    ["null", "null"],
  ])("%s", (_label, text) => {
    expect(parseBackup(text).ok).toBe(false);
  });

  it("理由が分かるメッセージを返す", () => {
    const got = parseBackup('{"version":9,"plan":{},"blocks":[]}');

    expect(got.ok).toBe(false);
    if (got.ok) return;
    expect(got.message).toContain("9");
  });
});

describe("backupFileName", () => {
  it("いつの設定かを名前に残す", () => {
    expect(backupFileName("2026-09-14T01:23:45.000Z")).toBe("plan_2026-09-14.json");
  });
});
