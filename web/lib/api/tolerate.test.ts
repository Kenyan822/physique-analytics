import { describe, expect, it } from "vitest";

import { ApiError } from "./client";
import { tolerate } from "./tolerate";

describe("tolerate", () => {
  it("成功したら値を返す", async () => {
    const got = await tolerate(async () => ({ n: 1 }), [422]);

    expect(got).toEqual({ value: { n: 1 }, unavailable: null });
  });

  it("想定したステータスなら理由に変える", async () => {
    // 画面を落とさない。フェーズ未登録は「まだ出せない」であって異常ではない
    const got = await tolerate(async () => {
      throw new ApiError(422, {
        type: "about:blank",
        status: 422,
        title: "入力が仕様に合わない",
        errors: [{ field: "phases", message: "フェーズが1つも登録されていない" }],
      });
    }, [422]);

    expect(got.value).toBeNull();
    expect(got.unavailable).toBe("フェーズが1つも登録されていない");
  });

  it("errors が無ければ detail を使う", async () => {
    const got = await tolerate(async () => {
      throw new ApiError(503, {
        type: "about:blank",
        status: 503,
        title: "写真の保存先が未設定",
        detail: "R2_ACCOUNT_ID を設定する",
      });
    }, [503]);

    expect(got.unavailable).toBe("R2_ACCOUNT_ID を設定する");
  });

  it("想定していないステータスは投げ直す", async () => {
    // 500 を握りつぶすと、本当の障害が画面上は「未設定」に見える
    await expect(
      tolerate(async () => {
        throw new ApiError(500, { type: "about:blank", status: 500, title: "サーバ内部エラー" });
      }, [422]),
    ).rejects.toBeInstanceOf(ApiError);
  });

  it("ApiError 以外は投げ直す", async () => {
    await expect(
      tolerate(async () => {
        throw new Error("ネットワークが落ちている");
      }, [422]),
    ).rejects.toThrow("ネットワークが落ちている");
  });
});
