import { describe, expect, it } from "vitest";

import { NAV_ITEMS, isCurrent } from "./items";

describe("NAV_ITEMS", () => {
  it("表示名は日本語だけにする", () => {
    // CSV / transfer のような実装側の語も、英単語も、使う人には意味が無い
    for (const item of NAV_ITEMS) {
      expect(item.label).not.toMatch(/[A-Za-z]/);
    }
  });

  it("すべての項目に補足がある", () => {
    for (const item of NAV_ITEMS) {
      expect(item.hint.length).toBeGreaterThan(0);
    }
  });

  it("行き先が重複しない", () => {
    const hrefs = NAV_ITEMS.map((i) => i.href);

    expect(new Set(hrefs).size).toBe(hrefs.length);
  });

  it("記録（/log）は入れない", () => {
    // 主要な操作としてボタンで常に見えているので、メニューに出すと二重になる
    expect(NAV_ITEMS.some((i) => i.href === "/log")).toBe(false);
  });
});

describe("isCurrent", () => {
  it("完全一致で選択中になる", () => {
    expect(isCurrent("/meals", "/meals")).toBe(true);
  });

  it("配下のページでも選択中のまま", () => {
    expect(isCurrent("/settings/advanced", "/settings")).toBe(true);
  });

  it("**トップは完全一致だけ**", () => {
    // 前方一致にすると、どのページでもトップが選択中になる
    expect(isCurrent("/", "/")).toBe(true);
    expect(isCurrent("/meals", "/")).toBe(false);
  });

  it("前方の文字が同じだけの別ページは選ばない", () => {
    expect(isCurrent("/settings-backup", "/settings")).toBe(false);
  });
});
