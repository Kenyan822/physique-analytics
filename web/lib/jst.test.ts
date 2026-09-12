import { describe, expect, it } from "vitest";

import { formatJstDate, toJstDate, todayJst } from "./jst";

describe("toJstDate", () => {
  // UTC に変換すると日本時間の朝9時前が前日になる。ADR-0013 の肝
  it("JST の深夜でも当日の日付になる", () => {
    // 2026-09-13 01:00 JST = 2026-09-12 16:00 UTC
    const d = new Date("2026-09-12T16:00:00Z");
    expect(d.toISOString().slice(0, 10)).toBe("2026-09-12"); // 素朴にやると前日
    expect(toJstDate(d)).toBe("2026-09-13");
  });

  it("JST の朝8時59分も当日", () => {
    // 2026-09-13 08:59 JST = 2026-09-12 23:59 UTC
    expect(toJstDate(new Date("2026-09-12T23:59:00Z"))).toBe("2026-09-13");
  });

  it("JST の 23:59 は翌日にならない", () => {
    // 2026-09-13 23:59 JST = 2026-09-13 14:59 UTC
    expect(toJstDate(new Date("2026-09-13T14:59:00Z"))).toBe("2026-09-13");
  });

  it("年をまたぐ", () => {
    // 2027-01-01 00:30 JST = 2026-12-31 15:30 UTC
    expect(toJstDate(new Date("2026-12-31T15:30:00Z"))).toBe("2027-01-01");
  });
});

describe("todayJst", () => {
  it("YYYY-MM-DD の形になる", () => {
    expect(todayJst()).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });
});

describe("formatJstDate", () => {
  it("曜日つきで表示する", () => {
    expect(formatJstDate("2026-09-13")).toBe("9/13(日)");
    expect(formatJstDate("2026-09-12")).toBe("9/12(土)");
  });

  it("壊れた入力はそのまま返す", () => {
    expect(formatJstDate("なんか変")).toBe("なんか変");
  });
});
