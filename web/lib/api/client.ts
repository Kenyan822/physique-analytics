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
export type TemplateInput = components["schemas"]["TemplateInput"];
export type TemplateItem = components["schemas"]["TemplateItem"];
export type DailyMetrics = components["schemas"]["DailyMetrics"];
export type DailyMetricsInput = components["schemas"]["DailyMetricsInput"];
export type BodyMeasurement = components["schemas"]["BodyMeasurement"];
export type BodyMeasurementInput = components["schemas"]["BodyMeasurementInput"];
export type Meal = components["schemas"]["Meal"];
export type MealInput = components["schemas"]["MealInput"];
export type MealSlot = components["schemas"]["MealSlot"];
export type MealSuggestion = components["schemas"]["MealSuggestion"];
export type MealEstimate = components["schemas"]["MealEstimate"];
export type MealSource = components["schemas"]["MealSource"];
export type DailyTargets = components["schemas"]["DailyTargets"];
export type ManualTargets = components["schemas"]["ManualTargets"];
export type MealSet = components["schemas"]["MealSet"];
export type MealSetInput = components["schemas"]["MealSetInput"];
export type Plan = components["schemas"]["Plan"];
export type PlanInput = components["schemas"]["PlanInput"];
export type PlanPhase = components["schemas"]["PlanPhase"];
export type PlanBlock = components["schemas"]["PlanBlock"];
export type MonthlyTarget = components["schemas"]["MonthlyTarget"];
export type Contest = components["schemas"]["Contest"];
export type ContestInput = components["schemas"]["ContestInput"];
export type MonthlyTargets = components["schemas"]["MonthlyTargets"];
export type BloodTest = components["schemas"]["BloodTest"];
export type BloodTestInput = components["schemas"]["BloodTestInput"];
export type BloodTestItem = components["schemas"]["BloodTestItem"];
export type BodyPhoto = components["schemas"]["BodyPhoto"];
export type PhotoPose = components["schemas"]["PhotoPose"];
export type ImportError = components["schemas"]["ImportError"];
export type CsvResource = NonNullable<
  paths["/v1/export/csv"]["get"]["parameters"]["query"]
>["resource"];
export type ImportResult =
  paths["/v1/import/csv"]["post"]["responses"]["200"]["content"]["application/json"];
export type VolumeRange = components["schemas"]["VolumeRange"];
export type NutritionSettings = components["schemas"]["NutritionSettings"];
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
  /**
   * Supabase の JWT。無ければ Authorization を付けない（/health 用）。
   *
   * **関数を渡せる。** ログイン中ユーザーのトークンは Cookie から非同期に読むため、
   * クライアントを作る時点では確定していない。呼び出しごとに解決する。
   */
  token?: string | (() => Promise<string | undefined>);
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

/** token が関数なら呼んで解決する。 */
async function resolveToken(token: ClientOptions["token"]): Promise<string | undefined> {
  return typeof token === "function" ? await token() : token;
}

export function createClient({ baseUrl, token }: ClientOptions) {
  async function request<T>(
    method: string,
    path: string,
    opts: { query?: Record<string, QueryValue>; body?: unknown } = {},
  ): Promise<T> {
    const headers: Record<string, string> = {};
    const jwt = await resolveToken(token);
    if (jwt) headers.Authorization = `Bearer ${jwt}`;
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

  /**
   * CSV は JSON ではないので `request` を通さない。
   * `text/csv` を `res.json()` に食わせると、原因が「JSON を読めない」に化ける。
   */
  async function requestCsv(query: Record<string, QueryValue>): Promise<string> {
    const headers: Record<string, string> = {};
    const jwt = await resolveToken(token);
    if (jwt) headers.Authorization = `Bearer ${jwt}`;

    const res = await fetch(buildUrl(baseUrl, "/v1/export/csv", query), {
      method: "GET",
      headers,
      cache: "no-store",
    });
    if (!res.ok) throw await toApiError(res);

    return res.text();
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
    createTemplate: (body: TemplateInput) => request<Template>("POST", "/v1/templates", { body }),
    updateTemplate: (id: string, body: TemplateInput) =>
      request<Template>("PATCH", `/v1/templates/${seg(id)}`, { body }),
    deleteTemplate: (id: string) => request<void>("DELETE", `/v1/templates/${seg(id)}`),

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

    listMeals: (query: DateRangeQuery) => request<{ items: Meal[] }>("GET", "/v1/meals", { query }),
    createMeal: (body: MealInput) => request<Meal>("POST", "/v1/meals", { body }),
    updateMeal: (id: string, body: MealInput) =>
      request<Meal>("PATCH", `/v1/meals/${seg(id)}`, { body }),
    deleteMeal: (id: string) => request<void>("DELETE", `/v1/meals/${seg(id)}`),
    // 過去の記録がそのまま候補になる（要件 N-02）
    mealSuggestions: (query: { q?: string; limit?: number }) =>
      request<{ items: MealSuggestion[] }>("GET", "/v1/meals/suggestions", { query }),
    copyMeals: (body: { fromDate: string; toDate: string; slot?: MealSlot }) =>
      request<{ items: Meal[] }>("POST", "/v1/meals/copy", { body }),
    // その日の摂取目標と残量（要件 N-05）
    dailyTargets: (date: string) => request<DailyTargets>("GET", `/v1/targets/${seg(date)}`),

    // 手で決めた摂取目標（要件 N-05）。あるときは自動計算（A-02）より優先される
    manualTargets: () =>
      request<{ targets: ManualTargets | null }>("GET", "/v1/targets/manual"),
    putManualTargets: (body: ManualTargets) =>
      request<ManualTargets>("PUT", "/v1/targets/manual", { body }),
    deleteManualTargets: () => request<void>("DELETE", "/v1/targets/manual"),

    // 食事セット（要件 N-03）
    listMealSets: () => request<{ items: MealSet[] }>("GET", "/v1/meal-sets"),
    createMealSet: (body: MealSetInput) => request<MealSet>("POST", "/v1/meal-sets", { body }),
    deleteMealSet: (id: string) => request<void>("DELETE", `/v1/meal-sets/${seg(id)}`),
    applyMealSet: (id: string, body: { date: string; slot?: MealSlot }) =>
      request<{ items: Meal[] }>("POST", `/v1/meal-sets/${seg(id)}/apply`, { body }),

    getPlan: () => request<Plan>("GET", "/v1/plan"),
    putPlan: (body: PlanInput) => request<Plan>("PUT", "/v1/plan", { body }),

    // 大会（要件 P-04）
    listContests: () => request<{ items: Contest[] }>("GET", "/v1/contests"),
    createContest: (body: ContestInput) => request<Contest>("POST", "/v1/contests", { body }),
    deleteContest: (id: string) => request<void>("DELETE", `/v1/contests/${seg(id)}`),

    // 計画ブロック（要件 P-02）
    listPlanBlocks: () => request<{ items: PlanBlock[] }>("GET", "/v1/plan/blocks"),
    putPlanBlocks: (items: PlanBlock[]) =>
      request<{ items: PlanBlock[] }>("PUT", "/v1/plan/blocks", { body: { items } }),

    // 月次目標（要件 P-02 / P-03）
    monthlyTargets: (baseline?: "configured" | "measured") =>
      request<MonthlyTargets>("GET", "/v1/plan/monthly-targets", {
        query: baseline ? { baseline } : {},
      }),

    // 血液検査（要件 B-08）
    listBloodTests: () => request<{ items: BloodTest[] }>("GET", "/v1/blood-tests"),
    createBloodTest: (body: BloodTestInput) =>
      request<BloodTest>("POST", "/v1/blood-tests", { body }),
    deleteBloodTest: (id: string) => request<void>("DELETE", `/v1/blood-tests/${seg(id)}`),

    /**
     * 写真から PFC を推定する（要件 N-06）。
     *
     * multipart なので `request` を通さない。**Content-Type を自分で付けない**
     * （boundary が落ちる）。推定は保存されない。下書きが返るだけ。
     */
    estimateMeal: async (form: FormData): Promise<MealEstimate> => {
      const headers: Record<string, string> = {};
      const jwt = await resolveToken(token);
      if (jwt) headers.Authorization = `Bearer ${jwt}`;

      const res = await fetch(buildUrl(baseUrl, "/v1/meals/estimate"), {
        method: "POST",
        headers,
        body: form,
        cache: "no-store",
      });
      if (!res.ok) throw await toApiError(res);

      return (await res.json()) as MealEstimate;
    },

    // CSV の取り込みと書き出し（要件 I-01 / I-02）
    exportCsv: (query: { resource: CsvResource; from?: string; to?: string }) =>
      requestCsv({ ...query }),
    importCsv: async (form: FormData): Promise<ImportResult> => {
      const headers: Record<string, string> = {};
      const jwt = await resolveToken(token);
      if (jwt) headers.Authorization = `Bearer ${jwt}`;

      // **Content-Type を自分で付けない。** boundary が付かず、サーバが読めなくなる
      const res = await fetch(buildUrl(baseUrl, "/v1/import/csv"), {
        method: "POST",
        headers,
        body: form,
        cache: "no-store",
      });
      if (!res.ok) throw await toApiError(res);

      return (await res.json()) as ImportResult;
    },

    // 身体写真（要件 B-04 / B-07）
    listPhotos: (query: DateRangeQuery) =>
      request<{ items: BodyPhoto[] }>("GET", "/v1/photos", { query }),
  };
}

export type ApiClient = ReturnType<typeof createClient>;
