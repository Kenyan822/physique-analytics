import type { PlanPhase } from "@/lib/api/client";

/**
 * フェーズの重なりを探す（要件 P-01）。
 *
 * **重なるとその日の目標ペースが一意に決まらない。** 停滞判定の基準が
 * 変わるので、保存する前に画面で気づけるようにする。
 * サーバ側でも同じ検証をしている（二重にするのは、往復する前に直せる方が速いため）。
 */
export function findOverlap(phases: PlanPhase[]): [PlanPhase, PlanPhase] | null {
  for (let i = 0; i < phases.length; i++) {
    for (let j = i + 1; j < phases.length; j++) {
      if (overlaps(phases[i], phases[j])) return [phases[i], phases[j]];
    }
  }

  return null;
}

function overlaps(a: PlanPhase, b: PlanPhase): boolean {
  return a.startsOn <= b.endsOn && b.startsOn <= a.endsOn;
}

/** 期間が逆になっているフェーズを返す。 */
export function findInvalidPeriod(phases: PlanPhase[]): PlanPhase | null {
  return phases.find((p) => p.endsOn < p.startsOn) ?? null;
}

/** 並べ替え。フェーズは時系列で読むもの。 */
export function sortPhases(phases: PlanPhase[]): PlanPhase[] {
  return [...phases].sort((a, b) => a.startsOn.localeCompare(b.startsOn));
}

/**
 * 次のフェーズの初期値。**前のフェーズの翌日から1ヶ月**にする。
 * 期間を毎回手で埋めるのは面倒で、かつ重なりの原因になる。
 */
export function nextPhaseDefaults(phases: PlanPhase[], today: string): PlanPhase {
  const last = sortPhases(phases).at(-1);
  const startsOn = last ? addDays(last.endsOn, 1) : today;

  return { name: "", startsOn, endsOn: addDays(startsOn, 30), goalKgPerWeek: 0 };
}

/** YYYY-MM-DD を n 日ずらす。JST 固定なので UTC で計算してよい（ADR-0013） */
export function addDays(date: string, n: number): string {
  const [y, m, d] = date.split("-").map(Number);

  return new Date(Date.UTC(y, m - 1, d + n)).toISOString().slice(0, 10);
}
