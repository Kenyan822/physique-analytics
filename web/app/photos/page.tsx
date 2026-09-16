import { SiteHeader } from "@/app/SiteHeader";
import { serverApi } from "@/lib/api/server";
import { tolerate } from "@/lib/api/tolerate";

import { PhotoTimeline } from "./PhotoTimeline";

export const metadata = { title: "写真 | physique" };

export default async function PhotosPage() {
  // R2 未設定の 503 は「まだ使えない」であって異常ではない（ADR-0008）
  const { value, unavailable } = await tolerate(() => serverApi().listPhotos({}), [503]);

  return (
    <>
      <SiteHeader title="写真" />

      <main className="mx-auto w-full max-w-6xl">
        <div className="px-4 py-4 lg:px-6">
          {unavailable ? (
            <div className="rounded-2xl border border-warn/40 bg-warn/10 p-4">
              <p className="text-sm font-medium text-warn">写真の保存先が未設定</p>
              <p className="mt-1 text-xs text-warn/90">{unavailable}</p>
            </div>
          ) : (
            <PhotoTimeline photos={value?.items ?? []} />
          )}
        </div>
      </main>
    </>
  );
}

/**
 * **503 でページを落とさない。** 保存先（R2）は課金の判断が要るので
 * 未設定のまま動く（ADR-0008）。設定方法はサーバが detail で返してくる。
 */
