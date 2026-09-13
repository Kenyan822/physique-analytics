"use server";

import { revalidatePath } from "next/cache";

import { ApiError, type WorkoutSession } from "@/lib/api/client";
import { serverApi } from "@/lib/api/server";

export type LastPerformance = Awaited<ReturnType<ReturnType<typeof serverApi>["lastPerformance"]>>;

export type RecordSetResult =
  { ok: true; session: WorkoutSession } | { ok: false; message: string };

export type EditSetResult = { ok: true } | { ok: false; message: string };

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
  /** クライアント生成の UUID。再送が冪等になり、削除・修正にも使える（要件 T-07 / T-10） */
  id?: string;
}): Promise<RecordSetResult> {
  const api = serverApi();

  try {
    const session = await ensureSession(api, input.date);

    await api.createWorkoutSet(session.id, {
      id: input.id,
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

/**
 * 記録したセットを直す（要件 T-10）。
 *
 * **オフラインでは使えない。** サーバにまだ届いていないセットは
 * 消しようがないので、未送信のあいだは画面側でボタンを無効にする。
 */
export async function updateSet(
  setId: string,
  input: { exerciseId: string; setNo: number; weightKg: number; reps: number; rir: number | null },
): Promise<EditSetResult> {
  return editSet(() => serverApi().updateWorkoutSet(setId, input));
}

/** 記録したセットを消す（要件 T-10）。 */
export async function deleteSet(setId: string): Promise<EditSetResult> {
  return editSet(() => serverApi().deleteWorkoutSet(setId));
}

async function editSet(fn: () => Promise<unknown>): Promise<EditSetResult> {
  try {
    await fn();
    revalidatePath("/");
    revalidatePath("/log");

    return { ok: true };
  } catch (e) {
    if (e instanceof ApiError) {
      // まだ同期されていないセットは 404 になる。原因が分かる文言に置き換える
      if (e.status === 404) {
        return { ok: false, message: "まだ送信されていない。送信が終われば直せる" };
      }

      return { ok: false, message: e.problem?.detail ?? e.problem?.title ?? e.message };
    }

    return { ok: false, message: e instanceof Error ? e.message : "直せなかった" };
  }
}
