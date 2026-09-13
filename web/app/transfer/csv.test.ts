import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

import { RESOURCES, exportFileName, resourceLabel, columnsOf } from "./csv";

describe("exportFileName", () => {
  it("期間が入っていればファイル名に含める", () => {
    expect(exportFileName("daily", "2026-09-01", "2026-09-30")).toBe(
      "daily_2026-09-01_2026-09-30.csv",
    );
  });

  it("期間が空なら all にする", () => {
    // 「いつの分か分からない CSV」が手元に残らないようにする
    expect(exportFileName("workouts", "", "")).toBe("workouts_all.csv");
  });

  it("開始だけ指定", () => {
    expect(exportFileName("measures", "2026-09-01", "")).toBe("measures_2026-09-01_all.csv");
  });

  it("終了だけ指定", () => {
    expect(exportFileName("daily", "", "2026-09-30")).toBe("daily_all_2026-09-30.csv");
  });
});

describe("columnsOf", () => {
  it("api/internal/csvio の列と同じ並びを返す", () => {
    expect(columnsOf("workouts")).toEqual([
      "date",
      "exercise",
      "set_no",
      "weight_kg",
      "reps",
      "rir",
    ]);
  });

  it("daily は note まで持つ", () => {
    const cols = columnsOf("daily");

    expect(cols[0]).toBe("date");
    expect(cols.at(-1)).toBe("note");
    expect(cols).toContain("hrv_ms");
  });

  it("measures は8箇所＋date", () => {
    expect(columnsOf("measures")).toHaveLength(9);
  });
});

describe("resourceLabel", () => {
  it("日本語にする", () => {
    expect(resourceLabel("daily")).toBe("日次");
    expect(resourceLabel("workouts")).toBe("トレーニング");
    expect(resourceLabel("measures")).toBe("周囲長");
  });
});

describe("RESOURCES", () => {
  it("3種類", () => {
    expect(RESOURCES).toEqual(["daily", "workouts", "measures"]);
  });
});

/**
 * 列は Go 側（`api/internal/csvio/csvio.go`）が正。コメントで「合わせてある」と
 * 書くだけだと必ずずれるので、実物を読んで突き合わせる。
 */
describe("Go の header と一致する", () => {
  // vitest は web/ を起点に走る（package.json の test）
  const go = readFileSync(resolve(process.cwd(), "../api/internal/csvio/csvio.go"), "utf8");

  function headerInGo(name: string): string[] {
    const m = go.match(new RegExp(`${name}\\s*=\\s*\\[\\]string\\{([\\s\\S]*?)\\}`));
    if (!m) throw new Error(`${name} を Go 側で見つけられない`);

    return [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
  }

  it.each([
    ["daily", "dailyHeader"],
    ["workouts", "workoutHeader"],
    ["measures", "measureHeader"],
  ] as const)("%s", (resource, goName) => {
    expect(columnsOf(resource)).toEqual(headerInGo(goName));
  });
});
