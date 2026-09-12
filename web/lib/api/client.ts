/**
 * API クライアント。
 *
 * 型は `openapi.yaml` からの生成物（schema.gen.ts）が正。**手で型を書かない**
 * （ADR-0007）。ここに書くのは、生成された型を使って組み立てる薄い層だけ。
 */
import type { components, paths } from "./schema.gen";

export type Exercise = components["schemas"]["Exercise"];
export type ExerciseInput = components["schemas"]["ExerciseInput"];
export type MuscleGroup = components["schemas"]["MuscleGroup"];
export type WorkoutSession = components["schemas"]["WorkoutSession"];
export type WorkoutSessionInput = components["schemas"]["WorkoutSessionInput"];
export type WorkoutSet = components["schemas"]["WorkoutSet"];
export type WorkoutSetInput = components["schemas"]["WorkoutSetInput"];
export type Template = components["schemas"]["Template"];
export type DailyMetrics = components["schemas"]["DailyMetrics"];
export type DailyMetricsInput = components["schemas"]["DailyMetricsInput"];
export type BodyMeasurement = components["schemas"]["BodyMeasurement"];
export type BodyMeasurementInput = components["schemas"]["BodyMeasurementInput"];
export type Problem = components["schemas"]["Problem"];

type ListExercisesQuery = NonNullable<paths["/v1/exercises"]["get"]["parameters"]["query"]>;
type ListSessionsQuery = NonNullable<paths["/v1/workout-sessions"]["get"]["parameters"]["query"]>;
type DateRangeQuery = NonNullable<paths["/v1/daily"]["get"]["parameters"]["query"]>;
type LastPerformance =
  paths["/v1/exercises/{exerciseId}/last-performance"]["get"]["responses"]["200"]["content"]["application/json"];

/** API が返したエラー。RFC 7807 の Problem を持つ。 */
export class ApiError extends Error {
  readonly status: number;
  readonly problem?: Problem;

  constructor(status: number, problem?: Problem) {
    // メッセージだけ見ても何が起きたか分かるようにする
    super(problem?.title ? `${status}: ${problem.title}` : `HTTP ${status}`);
    this.name = "ApiError";
    this.status = status;
    this.problem = problem;
  }
}

export type ClientOptions = {
  baseUrl: string;
  /** Supabase の JWT。無ければ Authorization を付けない（/health 用） */
  token?: string;
};

type QueryValue = string | number | boolean | undefined | null;

function buildUrl(baseUrl: string, path: string, query?: Record<string, QueryValue>): string {
  // 末尾スラッシュがあると //v1/... になる
  const url = new URL(baseUrl.replace(/\/+$/, "") + path);

  for (const [key, value] of Object.entries(query ?? {})) {
    // 未指定は送らない。空文字を送ると「空で絞る」と解釈されうる
    if (value === undefined || value === null) continue;
    url.searchParams.set(key, String(value));
  }

  return url.toString();
}

async function toApiError(res: Response): Promise<ApiError> {
  // problem+json で返ってこないこともある（LB の 502 など）。
  // そこで落ちるとエラーの原因が「JSON を読めない」にすり替わる
  try {
    const contentType = res.headers.get("content-type") ?? "";
    if (contentType.includes("json")) {
      return new ApiError(res.status, (await res.json()) as Problem);
    }
  } catch {
    // 読めなければステータスだけで返す
  }

  return new ApiError(res.status);
}

export function createClient({ baseUrl, token }: ClientOptions) {
  async function request<T>(
    method: string,
    path: string,
    opts: { query?: Record<string, QueryValue>; body?: unknown } = {},
  ): Promise<T> {
    const headers: Record<string, string> = {};
    if (token) headers.Authorization = `Bearer ${token}`;
    if (opts.body !== undefined) headers["Content-Type"] = "application/json";

    const res = await fetch(buildUrl(baseUrl, path, opts.query), {
      method,
      headers,
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      // 記録は毎回最新が要る。キャッシュされると入力直後に古い値が出る
      cache: "no-store",
    });

    if (!res.ok) throw await toApiError(res);
    // 204 はボディが無い。読もうとすると例外になる
    if (res.status === 204) return undefined as T;

    return (await res.json()) as T;
  }

  /** パスパラメータは必ずエスケープする。id に / が入ると別のパスになる */
  const seg = (v: string) => encodeURIComponent(v);

  return {
    health: () => request<{ status: "ok"; time: string }>("GET", "/health"),

    listExercises: (query: ListExercisesQuery) =>
      request<{ items: Exercise[] }>("GET", "/v1/exercises", { query }),
    getExercise: (id: string) => request<Exercise>("GET", `/v1/exercises/${seg(id)}`),
    createExercise: (body: ExerciseInput) => request<Exercise>("POST", "/v1/exercises", { body }),
    updateExercise: (id: string, body: ExerciseInput) =>
      request<Exercise>("PATCH", `/v1/exercises/${seg(id)}`, { body }),
    deleteExercise: (id: string) => request<void>("DELETE", `/v1/exercises/${seg(id)}`),

    lastPerformance: (exerciseId: string) =>
      request<LastPerformance>("GET", `/v1/exercises/${seg(exerciseId)}/last-performance`),

    listWorkoutSessions: (query: ListSessionsQuery) =>
      request<{ items: WorkoutSession[]; nextCursor?: string | null }>(
        "GET",
        "/v1/workout-sessions",
        { query },
      ),
    getWorkoutSession: (id: string) =>
      request<WorkoutSession>("GET", `/v1/workout-sessions/${seg(id)}`),
    createWorkoutSession: (body: WorkoutSessionInput) =>
      request<WorkoutSession>("POST", "/v1/workout-sessions", { body }),
    deleteWorkoutSession: (id: string) =>
      request<void>("DELETE", `/v1/workout-sessions/${seg(id)}`),

    createWorkoutSet: (sessionId: string, body: WorkoutSetInput) =>
      request<WorkoutSet>("POST", `/v1/workout-sessions/${seg(sessionId)}/sets`, { body }),
    updateWorkoutSet: (setId: string, body: WorkoutSetInput) =>
      request<WorkoutSet>("PATCH", `/v1/workout-sets/${seg(setId)}`, { body }),
    deleteWorkoutSet: (setId: string) => request<void>("DELETE", `/v1/workout-sets/${seg(setId)}`),

    listTemplates: () => request<{ items: Template[] }>("GET", "/v1/templates"),

    listDailyMetrics: (query: DateRangeQuery) =>
      request<{ items: DailyMetrics[] }>("GET", "/v1/daily", { query }),
    getDailyMetrics: (date: string) => request<DailyMetrics>("GET", `/v1/daily/${seg(date)}`),
    putDailyMetrics: (body: DailyMetricsInput) =>
      request<DailyMetrics>("PUT", "/v1/daily", { body }),
    deleteDailyMetrics: (date: string) => request<void>("DELETE", `/v1/daily/${seg(date)}`),

    listMeasurements: (query: DateRangeQuery) =>
      request<{ items: BodyMeasurement[] }>("GET", "/v1/measurements", { query }),
    putMeasurement: (body: BodyMeasurementInput) =>
      request<BodyMeasurement>("PUT", "/v1/measurements", { body }),
    // 前回値のデフォルト表示（要件 B-03）。記録が無ければ measurement は無い
    latestMeasurement: () =>
      request<{ measurement?: BodyMeasurement | null }>("GET", "/v1/measurements/latest"),
  };
}

export type ApiClient = ReturnType<typeof createClient>;
