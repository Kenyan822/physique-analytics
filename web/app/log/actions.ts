"use server";

import { revalidatePath } from "next/cache";

import { ApiError, type WorkoutSession } from "@/lib/api/client";
import { serverApi } from "@/lib/api/server";

export type LastPerformance = Awaited<ReturnType<ReturnType<typeof serverApi>["lastPerformance"]>>;

export type RecordSetResult = { ok: true; session: WorkoutSession } | { ok: false; message: string };

/** 種目の前回実施内容を取る（要件 T-02）。 */
export async function loadLastPerformance(exerciseId: string): Promise<LastPerformance> {
  return serverApi().lastPerformance(exerciseId);
}

/**
 * セットを1本記録する。
 *
 * その日のセッションが無ければ作る。**クライアントが日付を送る**のは、
 * 「今日」が JST で決まるため（ADR-0013）。サーバの UTC で判断すると
 * 日本時間の朝9時前が前日になる。
 */
export async function recordSet(input: {
  date: string;
  exerciseId: string;
  setNo: number;
  weightKg: number;
  reps: number;
  rir: number | null;
}): Promise<RecordSetResult> {
  const api = serverApi();

  try {
    const session = await ensureSession(api, input.date);

    await api.createWorkoutSet(session.id, {
      exerciseId: input.exerciseId,
      setNo: input.setNo,
      weightKg: input.weightKg,
      reps: input.reps,
      rir: input.rir,
    });

    // 記録後の一覧を最新にする
    revalidatePath("/");

    return { ok: true, session };
  } catch (e) {
    if (e instanceof ApiError) {
      // API が返した理由をそのまま見せる。何が悪いか分からないと直せない
      return { ok: false, message: e.problem?.detail ?? e.problem?.title ?? e.message };
    }

    return { ok: false, message: e instanceof Error ? e.message : "記録できなかった" };
  }
}

/** その日のセッションを取るか、無ければ作る。 */
async function ensureSession(
  api: ReturnType<typeof serverApi>,
  date: string,
): Promise<WorkoutSession> {
  const { items } = await api.listWorkoutSessions({ from: date, to: date, limit: 1 });
  if (items.length > 0) return items[0];

  return api.createWorkoutSession({ date });
}
