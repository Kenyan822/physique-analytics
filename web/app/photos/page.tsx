import Link from "next/link";

import { ApiError, type BodyPhoto } from "@/lib/api/client";
import { serverApi } from "@/lib/api/server";

import { PhotoTimeline } from "./PhotoTimeline";

export const metadata = { title: "写真 | physique" };

export default async function PhotosPage() {
  const { photos, unavailable } = await load();

  return (
    <main className="mx-auto w-full max-w-md lg:max-w-4xl">
      <header className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-line bg-bg/95 px-4 py-3 backdrop-blur lg:px-6 lg:py-4">
        <h1 className="text-lg font-semibold lg:text-xl">写真</h1>
        <Link
          href="/"
          className="pressable rounded-full border border-line px-3 py-1.5 text-sm text-muted"
        >
          今日の記録
        </Link>
      </header>

      <div className="px-4 py-4 lg:px-6">
        {unavailable ? (
          <div className="rounded-2xl border border-warn/40 bg-warn/10 p-4">
            <p className="text-sm font-medium text-warn">写真の保存先が未設定</p>
            <p className="mt-1 text-xs text-warn/90">{unavailable}</p>
          </div>
        ) : (
          <PhotoTimeline photos={photos} />
        )}
      </div>
    </main>
  );
}

/**
 * **503 でページを落とさない。** 保存先（R2）は課金の判断が要るので
 * 未設定のまま動く（ADR-0008）。設定方法はサーバが detail で返してくる。
 */
async function load(): Promise<{ photos: BodyPhoto[]; unavailable: string | null }> {
  try {
    const { items } = await serverApi().listPhotos({});

    return { photos: items, unavailable: null };
  } catch (e) {
    if (e instanceof ApiError && e.status === 503) {
      return { photos: [], unavailable: e.problem?.detail ?? "R2 の設定が要る" };
    }

    throw e;
  }
}
