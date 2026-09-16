import { describe, expect, it } from "vitest";

import { defaultSlot } from "./slot";

/** JST の時刻を Date にする。テストのホストのタイムゾーンに依存させない */
function jst(hour: number, minute = 0): Date {
  return new Date(Date.UTC(2026, 8, 16, hour - 9, minute));
}

describe("食事の区分を時刻から推測する", () => {
  it("時間帯ごとに選ぶ", () => {
    expect(defaultSlot(jst(7))).toBe("朝食");
    expect(defaultSlot(jst(12))).toBe("昼食");
    expect(defaultSlot(jst(19))).toBe("夕食");
    expect(defaultSlot(jst(22))).toBe("間食");
  });

  it("境界", () => {
    expect(defaultSlot(jst(9, 59))).toBe("朝食");
    expect(defaultSlot(jst(10))).toBe("昼食");
    expect(defaultSlot(jst(14, 59))).toBe("昼食");
    expect(defaultSlot(jst(15))).toBe("夕食");
    expect(defaultSlot(jst(20, 59))).toBe("夕食");
    expect(defaultSlot(jst(21))).toBe("間食");
  });

  // **これが #175 の本体。** getHours() はホストのタイムゾーンで評価されるので、
  // UTC で動くサーバと JST のブラウザで別の答えになり hydration が食い違う
  it("**ホストのタイムゾーンで答えが変わらない**", () => {
    // JST 19:18 = UTC 10:18。UTC の getHours() なら 10 で「昼食」になってしまう
    expect(defaultSlot(new Date("2026-09-16T10:18:00Z"))).toBe("夕食");
    // JST 22:00 = UTC 13:00。UTC の getHours() なら 13 で「昼食」になってしまう
    expect(defaultSlot(new Date("2026-09-16T13:00:00Z"))).toBe("間食");
    // JST 8:00 = UTC 23:00（前日）。UTC の getHours() なら 23 で「間食」になってしまう
    expect(defaultSlot(new Date("2026-09-15T23:00:00Z"))).toBe("朝食");
    // JST 翌 0:30 = UTC 15:30。日付をまたいでも JST の「時」で見る
    expect(defaultSlot(new Date("2026-09-16T15:30:00Z"))).toBe("朝食");
  });
});
