import { cleanup, render, screen } from "@testing-library/react";
import type React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api/client";

/**
 * **全画面が開くことを固定する。**
 *
 * 画面が落ちる不具合は、たいてい「まだ設定が済んでいない」状態で起きる
 * （フェーズ未登録の 422、R2 未設定の 503）。空の DB を想定した応答を返して、
 * どのページも開けることを確かめる。
 */
const api = {
  listWorkoutSessions: vi.fn(),
  listExercises: vi.fn(),
  listTemplates: vi.fn(),
  listContests: vi.fn(),
  getPlan: vi.fn(),
  listPlanBlocks: vi.fn(),
  monthlyTargets: vi.fn(),
  listDailyMetrics: vi.fn(),
  listMeasurements: vi.fn(),
  listMeals: vi.fn(),
  dailyTargets: vi.fn(),
  listMealSets: vi.fn(),
  listBloodTests: vi.fn(),
  listPhotos: vi.fn(),
};

vi.mock("@/lib/api/server", () => ({ serverApi: () => api }));

/**
 * Server Actions は**モックしない。**
 *
 * export を列挙すると、アクションを1つ足すたびにこのテストが「未定義の export」で
 * 落ちる。代わりに**その下層だけ差し替える** —— `server-only`（クライアントから
 * import すると落ちる番人）と、Next のランタイムが要る関数。
 * これでアクション本体はそのまま読み込める。
 */
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => ({ revalidatePath: vi.fn(), revalidateTag: vi.fn() }));
vi.mock("next/navigation", () => ({ redirect: vi.fn(), notFound: vi.fn() }));

const emptyPlan = {
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

/** 何も設定していない、まっさらな状態を模す */
function asEmptyDatabase() {
  api.listWorkoutSessions.mockResolvedValue({ items: [] });
  api.listExercises.mockResolvedValue({ items: [] });
  api.listTemplates.mockResolvedValue({ items: [] });
  api.listContests.mockResolvedValue({ items: [] });
  api.getPlan.mockResolvedValue(emptyPlan);
  api.listPlanBlocks.mockResolvedValue({ items: [] });
  api.listDailyMetrics.mockResolvedValue({ items: [] });
  api.listMeasurements.mockResolvedValue({ items: [] });
  api.listMeals.mockResolvedValue({ items: [] });
  api.listMealSets.mockResolvedValue({ items: [] });
  api.listBloodTests.mockResolvedValue({ items: [] });

  // フェーズ未登録だと目標を出せない
  const unprocessable = new ApiError(422, {
    type: "about:blank",
    status: 422,
    title: "入力が仕様に合わない",
    errors: [{ field: "phases", message: "フェーズが1つも登録されていない" }],
  });
  api.dailyTargets.mockRejectedValue(unprocessable);
  api.monthlyTargets.mockRejectedValue(unprocessable);

  // 写真は保存先が未設定
  api.listPhotos.mockRejectedValue(
    new ApiError(503, {
      type: "about:blank",
      status: 503,
      title: "写真の保存先が未設定",
      detail: "R2_ACCOUNT_ID を設定する",
    }),
  );
}

/**
 * **見出しの文言では判定しない。** コピーを直すたびにテストが壊れると、
 * 「開けるか」を見たいのに文言の写経になる。見出しが1つ以上あることだけ見る。
 */
const PAGES = [
  ["/", () => import("@/app/page")],
  ["/log", () => import("@/app/log/page")],
  ["/meals", () => import("@/app/meals/page")],
  ["/body", () => import("@/app/body/page")],
  ["/plan", () => import("@/app/plan/page")],
  ["/photos", () => import("@/app/photos/page")],
  ["/blood", () => import("@/app/blood/page")],
  ["/settings", () => import("@/app/settings/page")],
  ["/transfer", () => import("@/app/transfer/page")],
] as const;

beforeEach(asEmptyDatabase);
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("まっさらな状態でも全画面が開く", () => {
  it.each(PAGES)("%s", async (_path, load) => {
    // ページごとに引数の有無が違う（searchParams を取るものだけ受け取る）
    const Page = (await load()).default as (p?: unknown) => Promise<React.ReactElement>;
    render(await Page({ searchParams: Promise.resolve({}) }));

    expect(screen.getAllByRole("heading").length).toBeGreaterThan(0);
  });
});

describe("設定が済んでいない理由が画面に出る", () => {
  it("食事: 目標を出せない理由", async () => {
    const Page = (await import("@/app/meals/page")).default;
    render(await Page());

    expect(screen.getByText(/フェーズが1つも登録されていない/)).toBeDefined();
  });

  it("写真: 保存先の設定方法", async () => {
    const Page = (await import("@/app/photos/page")).default;
    render(await Page());

    expect(screen.getByText(/R2_ACCOUNT_ID/)).toBeDefined();
  });
});
