export type ExerciseSummary = {
  exerciseId: string;
  sets: number;
  tonnageKg: number;
};

/**
 * 今日の記録を種目ごとに畳む（セット数とトン数）。
 * 並びは最初に記録した種目から。ジムでは「今なにを何セットやったか」を
 * 一覧の下まで数えずに見たい。
 */
export function summarizeByExercise(
  sets: readonly { exerciseId: string; weightKg: number; reps: number }[],
): ExerciseSummary[] {
  const byId = new Map<string, ExerciseSummary>();
  for (const s of sets) {
    const cur = byId.get(s.exerciseId) ?? { exerciseId: s.exerciseId, sets: 0, tonnageKg: 0 };
    cur.sets += 1;
    cur.tonnageKg += s.weightKg * s.reps;
    byId.set(s.exerciseId, cur);
  }

  return [...byId.values()];
}
