import { describe, expect, it } from "vitest";

import type { PlanPhase } from "@/lib/api/client";

import { addDays, findInvalidPeriod, findOverlap, nextPhaseDefaults, sortPhases } from "./phases";

function phase(name: string, startsOn: string, endsOn: string): PlanPhase {
  return { name, startsOn, endsOn, goalKgPerWeek: 0 };
}

describe("findOverlap", () => {
  it("重なりを見つける", () => {
    const got = findOverlap([
      phase("A", "2026-10-01", "2026-10-31"),
      phase("B", "2026-10-15", "2026-11-15"),
    ]);

    expect(got?.map((p) => p.name)).toEqual(["A", "B"]);
  });

  it("端が接していても重なりとみなす", () => {
    // 10-31 が両方に属すると、その日の目標ペースが決まらない
    const got = findOverlap([
      phase("A", "2026-10-01", "2026-10-31"),
      phase("B", "2026-10-31", "2026-11-30"),
    ]);

    expect(got).not.toBeNull();
  });

  it("連続していれば重ならない", () => {
    const got = findOverlap([
      phase("A", "2026-10-01", "2026-10-31"),
      phase("B", "2026-11-01", "2026-11-30"),
    ]);

    expect(got).toBeNull();
  });

  it("1件なら重ならない", () => {
    expect(findOverlap([phase("A", "2026-10-01", "2026-10-31")])).toBeNull();
  });
});

describe("findInvalidPeriod", () => {
  it("終了日が開始日より前なら返す", () => {
    expect(findInvalidPeriod([phase("A", "2026-10-31", "2026-10-01")])?.name).toBe("A");
  });

  it("同じ日は許す", () => {
    expect(findInvalidPeriod([phase("A", "2026-10-01", "2026-10-01")])).toBeNull();
  });
});

describe("sortPhases", () => {
  it("開始日の昇順にする", () => {
    const got = sortPhases([
      phase("B", "2026-11-01", "2026-11-30"),
      phase("A", "2026-10-01", "2026-10-31"),
    ]);

    expect(got.map((p) => p.name)).toEqual(["A", "B"]);
  });

  it("元の配列を書き換えない", () => {
    const input = [phase("B", "2026-11-01", "2026-11-30"), phase("A", "2026-10-01", "2026-10-31")];
    sortPhases(input);

    expect(input[0].name).toBe("B");
  });
});

describe("nextPhaseDefaults", () => {
  it("前のフェーズの翌日から始める", () => {
    const got = nextPhaseDefaults([phase("A", "2026-10-01", "2026-10-31")], "2026-09-13");

    expect(got.startsOn).toBe("2026-11-01");
    expect(got.endsOn).toBe("2026-12-01");
  });

  it("フェーズが無ければ今日から", () => {
    expect(nextPhaseDefaults([], "2026-09-13").startsOn).toBe("2026-09-13");
  });
});

describe("addDays", () => {
  it("月をまたぐ", () => {
    expect(addDays("2026-10-31", 1)).toBe("2026-11-01");
  });

  it("年をまたぐ", () => {
    expect(addDays("2026-12-31", 1)).toBe("2027-01-01");
  });

  it("戻せる", () => {
    expect(addDays("2026-01-01", -1)).toBe("2025-12-31");
  });
});
