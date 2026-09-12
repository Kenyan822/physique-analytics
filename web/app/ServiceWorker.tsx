"use client";

import { useEffect } from "react";

/**
 * Service Worker を登録する（要件 T-07）。
 *
 * **開発中は登録しない。** キャッシュが効くと、コードを変えても古い画面が
 * 出続けて原因が分からなくなる。
 */
export function ServiceWorker() {
  useEffect(() => {
    if (process.env.NODE_ENV !== "production") return;
    if (!("serviceWorker" in navigator)) return;

    // 登録の失敗でアプリが落ちてはいけない。オフラインが効かないだけ
    navigator.serviceWorker.register("/sw.js").catch(() => {});
  }, []);

  return null;
}
