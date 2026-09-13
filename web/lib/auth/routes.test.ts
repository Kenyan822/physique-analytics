import { describe, expect, it } from "vitest";

import { requiresLogin } from "./routes";

describe("requiresLogin", () => {
  it.each(["/", "/log", "/meals", "/body", "/plan", "/blood", "/photos", "/settings", "/transfer"])(
    "%s はログインが要る",
    (p) => {
      expect(requiresLogin(p)).toBe(true);
    },
  );

  it.each(["/login", "/auth/callback"])("%s はログイン無しで開ける", (p) => {
    expect(requiresLogin(p)).toBe(false);
  });

  it("Next の内部パスは対象外", () => {
    // ここで弾かないと CSS や JS の取得までリダイレクトされて画面が壊れる
    expect(requiresLogin("/_next/static/chunks/main.js")).toBe(false);
  });

  it.each(["/favicon.ico", "/sw.js", "/manifest.json"])("静的ファイル %s は対象外", (p) => {
    expect(requiresLogin(p)).toBe(false);
  });

  it("login で始まるだけの別パスは保護する", () => {
    // /login-history のようなパスが素通りしないこと
    expect(requiresLogin("/login-history")).toBe(true);
  });
});
