import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { NAV_ITEMS } from "@/lib/nav/items";

const pathname = vi.fn(() => "/meals");
vi.mock("next/navigation", () => ({ usePathname: () => pathname() }));
vi.mock("./login/actions", () => ({ logout: vi.fn() }));

const { SiteHeader } = await import("./SiteHeader");

afterEach(() => {
  cleanup();
  pathname.mockReturnValue("/meals");
});

describe("SiteHeader", () => {
  it("見出しと日付を出す", () => {
    render(<SiteHeader title="食事" subtitle="9/16(水)" />);

    expect(screen.getByRole("heading", { name: /食事/ })).toBeDefined();
    expect(screen.getByText("9/16(水)")).toBeDefined();
  });

  it("**どの画面からでも他の画面へ行ける**", () => {
    render(<SiteHeader title="食事" />);

    // 以前はトップにしかナビが無く、食事から体組成へ移るのに一度戻る必要があった
    for (const item of NAV_ITEMS) {
      expect(screen.getAllByRole("link", { name: item.label }).length).toBeGreaterThan(0);
    }
  });

  it("いま開いている画面が分かる", () => {
    render(<SiteHeader title="食事" />);

    const current = screen.getAllByRole("link", { name: "食事" });
    expect(current.some((el) => el.getAttribute("aria-current") === "page")).toBe(true);
  });

  it("トップにいるとき、他の画面が選択中にならない", () => {
    pathname.mockReturnValue("/");
    render(<SiteHeader title="今日" />);

    const meals = screen.getAllByRole("link", { name: "食事" });
    expect(meals.every((el) => el.getAttribute("aria-current") === null)).toBe(true);
  });

  it("記録するボタンが常に見える", () => {
    render(<SiteHeader title="食事" />);

    expect(screen.getByRole("link", { name: "記録する" })).toBeDefined();
  });

  it("記録画面では「記録する」を出さない", () => {
    // いま開いている画面へのボタンは意味が無い
    pathname.mockReturnValue("/log");
    render(<SiteHeader title="記録" />);

    expect(screen.queryByRole("link", { name: "記録する" })).toBeNull();
  });
});

describe("メニュー（ハンバーガー）", () => {
  it("開く前は閉じている", () => {
    render(<SiteHeader title="食事" />);

    expect(
      screen.getByRole("button", { name: "メニューを開く" }).getAttribute("aria-expanded"),
    ).toBe("false");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("押すと開く", () => {
    render(<SiteHeader title="食事" />);
    fireEvent.click(screen.getByRole("button", { name: "メニューを開く" }));

    expect(screen.getByRole("dialog", { name: "メニュー" })).toBeDefined();
    expect(screen.getByRole("button", { name: "メニューを閉じる" })).toBeDefined();
  });

  it("もう一度押すと閉じる", () => {
    render(<SiteHeader title="食事" />);
    fireEvent.click(screen.getByRole("button", { name: "メニューを開く" }));
    fireEvent.click(screen.getByRole("button", { name: "メニューを閉じる" }));

    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("Esc で閉じる", () => {
    render(<SiteHeader title="食事" />);
    fireEvent.click(screen.getByRole("button", { name: "メニューを開く" }));
    fireEvent.keyDown(document, { key: "Escape" });

    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("開くと補足が出る", () => {
    render(<SiteHeader title="食事" />);
    fireEvent.click(screen.getByRole("button", { name: "メニューを開く" }));

    expect(screen.getByText(NAV_ITEMS[1].hint)).toBeDefined();
  });

  it("メニューの中にログアウトがある", () => {
    render(<SiteHeader title="食事" />);
    fireEvent.click(screen.getByRole("button", { name: "メニューを開く" }));

    const sheet = screen.getByRole("dialog", { name: "メニュー" });
    expect(within(sheet).getByRole("button", { name: "ログアウト" })).toBeDefined();
  });

  it("**広い画面用のログアウトがメニューの外にもある**", () => {
    // ナビが横に出ているときはハンバーガーを出さないので、
    // メニューの中だけにあるとログアウトできなくなる
    render(<SiteHeader title="食事" />);

    const outside = screen
      .getAllByRole("button", { name: "ログアウト" })
      .filter((el) => el.closest('[role="dialog"]') === null);
    expect(outside.length).toBe(1);
  });
});
