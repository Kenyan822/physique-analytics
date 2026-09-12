import type { BodyPhoto, PhotoPose } from "@/lib/api/client";

/**
 * 並べる順。**撮った順ではなく決まった順にする。**
 * 日付をまたいで見比べるので、同じ位置に同じ向きが来ないと比較にならない。
 */
export const POSES: PhotoPose[] = ["front", "side", "back"];

const LABELS: Record<PhotoPose, string> = {
  front: "正面",
  side: "側面",
  back: "背面",
};

export type DateGroup = { date: string; photos: BodyPhoto[] };

export function poseLabel(pose: PhotoPose): string {
  return LABELS[pose];
}

/** 撮影日ごとにまとめる（要件 B-07）。新しい日が先。 */
export function groupByDate(photos: BodyPhoto[]): DateGroup[] {
  const byDate = new Map<string, BodyPhoto[]>();
  for (const p of photos) {
    const list = byDate.get(p.date) ?? [];
    list.push(p);
    byDate.set(p.date, list);
  }

  return [...byDate]
    .sort(([a], [b]) => b.localeCompare(a))
    .map(([date, list]) => ({
      date,
      photos: [...list].sort((x, y) => POSES.indexOf(x.pose) - POSES.indexOf(y.pose)),
    }));
}
