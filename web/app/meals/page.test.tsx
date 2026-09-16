import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ApiError, type DailyTargets } from "@/lib/api/client";

/**
 * **食事画面が開けなくなった不具合の回帰テスト。**
 *
 * フェーズ未登録だと `GET /v1/targets/{date}` が 422 を返す。これを捕まえて
 * いなかったため、画面全体が落ちて記録そのものができなかった。
 */
const api = {
  listMeals: vi.fn(),
  dailyTargets: vi.fn(),
  listMealSets: vi.fn(),
};

vi.mock("@/lib/api/server", () => ({ serverApi: () => api }));
// アクションは列挙せず、下層だけ差し替える（app/__tests__/pages.test.tsx と同じ理由）
vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => ({ revalidatePath: vi.fn(), revalidateTag: vi.fn() }));

const targets: DailyTargets = {
  date: "2026-09-16",
  consumed: { kcal: 0, proteinG: 0, fatG: 0, carbG: 0 },
};

async function renderPage() {
  const MealsPage = (await import("./page")).default;
  render(await MealsPage());
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("食事画面", () => {
  it("目標が取れれば普通に開く", async () => {
    api.listMeals.mockResolvedValue({ items: [] });
    api.listMealSets.mockResolvedValue({ items: [] });
    api.dailyTargets.mockResolvedValue(targets);

    await renderPage();

    expect(screen.getByRole("heading", { name: /食事/ })).toBeDefined();
  });

  it("**目標が 422 でも画面が開く**", async () => {
    api.listMeals.mockResolvedValue({ items: [] });
    api.listMealSets.mockResolvedValue({ items: [] });
    api.dailyTargets.mockRejectedValue(
      new ApiError(422, {
        type: "about:blank",
        status: 422,
        title: "入力が仕様に合わない",
        errors: [{ field: "phases", message: "フェーズが1つも登録されていない" }],
      }),
    );

    await renderPage();

    // 記録の入力欄は使えたままであること
    expect(screen.getByLabelText("食べたもの")).toBeDefined();
  });

  it("目標を出せない理由と、直しに行く導線が出る", async () => {
    api.listMeals.mockResolvedValue({ items: [] });
    api.listMealSets.mockResolvedValue({ items: [] });
    api.dailyTargets.mockRejectedValue(
      new ApiError(422, {
        type: "about:blank",
        status: 422,
        title: "入力が仕様に合わない",
        errors: [{ field: "phases", message: "フェーズが1つも登録されていない" }],
      }),
    );

    await renderPage();

    expect(screen.getByText(/フェーズが1つも登録されていない/)).toBeDefined();
    expect(screen.getByRole("link", { name: "設定へ" })).toBeDefined();
  });

  it("500 は握りつぶさない", async () => {
    // 本当の障害が「未設定」に見えると原因を見誤る
    api.listMeals.mockResolvedValue({ items: [] });
    api.listMealSets.mockResolvedValue({ items: [] });
    api.dailyTargets.mockRejectedValue(
      new ApiError(500, { type: "about:blank", status: 500, title: "サーバ内部エラー" }),
    );

    await expect(renderPage()).rejects.toBeInstanceOf(ApiError);
  });
});
