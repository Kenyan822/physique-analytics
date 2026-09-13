import { describe, expect, it } from "vitest";

import { endOfMonth, sortContests } from "./contest";

import type { Contest } from "@/lib/api/client";

function contest(id: string, heldOn: string): Contest {
  return {
    id,
    heldOn,
    category: id,
    targetBfPct: 11,
    createdAt: "2026-09-13T00:00:00Z",
    updatedAt: "2026-09-13T00:00:00Z",
  };
}

describe("endOfMonth", () => {
  it("月末を返す", () => {
    expect(endOfMonth("2026-09-13")).toBe("2026-09-30");
  });

  it("31日の月", () => {
    expect(endOfMonth("2026-10-01")).toBe("2026-10-31");
  });

  it("うるう年の2月", () => {
    expect(endOfMonth("2028-02-05")).toBe("2028-02-29");
  });

  it("平年の2月", () => {
    expect(endOfMonth("2027-02-05")).toBe("2027-02-28");
  });

  it("12月は年をまたがない", () => {
    expect(endOfMonth("2026-12-25")).toBe("2026-12-31");
  });
});

describe("sortContests", () => {
  it("開催日の昇順に並べる", () => {
    const got = sortContests([
      contest("b", "2027-07-04"),
      contest("a", "2026-11-23"),
      contest("c", "2028-03-01"),
    ]);

    expect(got.map((c) => c.id)).toEqual(["a", "b", "c"]);
  });

  it("元の配列を書き換えない", () => {
    const list = [contest("b", "2027-07-04"), contest("a", "2026-11-23")];
    sortContests(list);

    expect(list.map((c) => c.id)).toEqual(["b", "a"]);
  });
});
