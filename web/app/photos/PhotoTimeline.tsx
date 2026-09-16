"use client";

import { useState } from "react";

import type { BodyPhoto, PhotoPose } from "@/lib/api/client";

import { POSES, groupByDate, poseLabel } from "./timeline";

type Props = { photos: BodyPhoto[] };

/**
 * 写真のタイムライン（要件 B-07）。
 *
 * **向きで絞り込めるようにしてある。** 時系列の比較は向きを揃えないと
 * 成立しない。正面だけを並べたいのが普通の見方。
 */
export function PhotoTimeline({ photos }: Props) {
  const [pose, setPose] = useState<PhotoPose | null>(null);

  const shown = pose ? photos.filter((p) => p.pose === pose) : photos;
  const groups = groupByDate(shown);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-2 overflow-x-auto">
        <Chip active={pose === null} onClick={() => setPose(null)}>
          すべて
        </Chip>
        {POSES.map((p) => (
          <Chip key={p} active={pose === p} onClick={() => setPose(pose === p ? null : p)}>
            {poseLabel(p)}
          </Chip>
        ))}
      </div>

      {groups.length === 0 && (
        <p className="rounded-2xl border border-line bg-surface p-8 text-center text-sm text-muted">
          この向きの写真はまだ無い
        </p>
      )}

      {groups.map((g) => (
        <section key={g.date} className="rounded-2xl border border-line bg-surface p-4">
          <h2 className="tnum mb-3 text-sm font-semibold">{g.date}</h2>
          {/*
           * 狭い画面は3枚（正面・側面・背面が1行に収まる）。
           * 広い画面で3枚のままだと1枚が大きすぎて、日付をまたいで見比べられない
           */}
          <ul className="grid grid-cols-3 gap-2 md:grid-cols-6 lg:gap-4 xl:grid-cols-9">
            {g.photos.map((p) => (
              <li key={p.id} className="flex flex-col gap-1.5">
                <Frame photo={p} />
                <span className="text-[11px] text-muted">{poseLabel(p.pose)}</span>
                {p.note && <span className="text-[11px] text-muted">{p.note}</span>}
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}

/**
 * 1枚。**URL が無いことがある。**
 * 署名付きURLの発行に失敗しても一覧はメタデータだけで返る（handler の withURLs）。
 * そのとき枠ごと消すと「撮ったはずの写真が無い」ように見える。
 */
function Frame({ photo }: { photo: BodyPhoto }) {
  if (!photo.url) {
    return (
      <div className="flex aspect-[3/4] items-center justify-center rounded-xl border border-line bg-surface-2 p-2 text-center text-[11px] text-muted">
        画像を読めなかった
      </div>
    );
  }

  return (
    <a href={photo.url} target="_blank" rel="noreferrer" className="pressable block">
      {/*
       * next/image を使わない。URL は短期の署名付きで毎回変わるため、
       * 最適化のキャッシュが効かないどころか、失効したURLを掴み続ける
       */}
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        src={photo.url}
        alt={`${photo.date} ${poseLabel(photo.pose)}`}
        loading="lazy"
        className="aspect-[3/4] w-full rounded-xl border border-line bg-surface-2 object-cover"
      />
    </a>
  );
}

function Chip({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={`pressable shrink-0 rounded-full border px-3.5 py-1.5 text-sm ${
        active
          ? "border-accent bg-accent font-semibold text-accent-ink"
          : "border-line bg-surface text-muted"
      }`}
    >
      {children}
    </button>
  );
}
