import { describe, expect, it } from "vitest";

import { safeNext } from "./next";

describe("safeNext", () => {
  it("サイト内のパスはそのまま", () => {
    expect(safeNext("/log")).toBe("/log");
    expect(safeNext("/settings")).toBe("/settings");
  });

  it("外部URLは弾く", () => {
    expect(safeNext("https://evil.example")).toBe("/");
    expect(safeNext("http://evil.example")).toBe("/");
  });

  it("プロトコル相対URLも弾く", () => {
    // //evil.example は外部サイトに飛ぶ
    expect(safeNext("//evil.example")).toBe("/");
  });

  it("空や未指定はトップ", () => {
    expect(safeNext("")).toBe("/");
    expect(safeNext(null)).toBe("/");
    expect(safeNext(undefined)).toBe("/");
  });

  it("スキームだけのものも弾く", () => {
    expect(safeNext("javascript:alert(1)")).toBe("/");
  });
});
