import { describe, expect, it } from "vitest";

import type { BodyPhoto, PhotoPose } from "@/lib/api/client";

import { POSES, groupByDate, poseLabel } from "./timeline";

function photo(date: string, pose: PhotoPose, id = `${date}-${pose}`): BodyPhoto {
  return {
    id,
    date,
    pose,
    mimeType: "image/jpeg",
    byteSize: 1024,
    createdAt: `${date}T00:00:00Z`,
    updatedAt: `${date}T00:00:00Z`,
  };
}

describe("groupByDate", () => {
  it("日付ごとにまとめる", () => {
    const got = groupByDate([
      photo("2028-04-01", "front"),
      photo("2028-04-01", "side"),
      photo("2028-03-01", "front"),
    ]);

    expect(got.map((g) => g.date)).toEqual(["2028-04-01", "2028-03-01"]);
    expect(got[0].photos).toHaveLength(2);
  });

  it("新しい日付が先", () => {
    // API は降順で返すが、絞り込んだ後も順序を保つことをここで担保する
    const got = groupByDate([photo("2028-03-01", "front"), photo("2028-04-01", "front")]);

    expect(got.map((g) => g.date)).toEqual(["2028-04-01", "2028-03-01"]);
  });

  it("同じ日の中は 正面 → 側面 → 背面 の順にする", () => {
    // 撮った順ではなく決まった順に並べないと、日付をまたいで見比べられない
    const got = groupByDate([
      photo("2028-04-01", "back"),
      photo("2028-04-01", "front"),
      photo("2028-04-01", "side"),
    ]);

    expect(got[0].photos.map((p) => p.pose)).toEqual(["front", "side", "back"]);
  });

  it("空なら空", () => {
    expect(groupByDate([])).toEqual([]);
  });
});

describe("poseLabel", () => {
  it("日本語にする", () => {
    expect(poseLabel("front")).toBe("正面");
    expect(poseLabel("side")).toBe("側面");
    expect(poseLabel("back")).toBe("背面");
  });
});

describe("POSES", () => {
  it("並べる順を持つ", () => {
    expect(POSES).toEqual(["front", "side", "back"]);
  });
});
