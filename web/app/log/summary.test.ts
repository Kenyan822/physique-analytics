import { describe, expect, it } from "vitest";

import { summarizeByExercise } from "./summary";

describe("summarizeByExercise", () => {
  it("種目ごとにセット数とトン数を畳む", () => {
    const got = summarizeByExercise([
      { exerciseId: "bench", weightKg: 60, reps: 8 },
      { exerciseId: "bench", weightKg: 62.5, reps: 6 },
      { exerciseId: "squat", weightKg: 80, reps: 5 },
    ]);
    expect(got).toEqual([
      { exerciseId: "bench", sets: 2, tonnageKg: 60 * 8 + 62.5 * 6 },
      { exerciseId: "squat", sets: 1, tonnageKg: 400 },
    ]);
  });

  it("並びは最初に記録した種目から（途中で戻っても順は変わらない）", () => {
    const got = summarizeByExercise([
      { exerciseId: "squat", weightKg: 80, reps: 5 },
      { exerciseId: "bench", weightKg: 60, reps: 8 },
      { exerciseId: "squat", weightKg: 85, reps: 3 },
    ]);
    expect(got.map((g) => g.exerciseId)).toEqual(["squat", "bench"]);
    expect(got[0].sets).toBe(2);
  });

  it("記録が無ければ空", () => {
    expect(summarizeByExercise([])).toEqual([]);
  });
});
