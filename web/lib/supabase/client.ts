"use client";

import { createBrowserClient } from "@supabase/ssr";

/**
 * ブラウザ側の Supabase クライアント。ログイン画面だけが使う。
 *
 * anon key は**公開前提**のもの（ブラウザに載る）。これ単体では何も読めない
 * —— DB は RLS で閉じてある（マイグレーション 000012）。
 */
export function supabaseBrowser() {
  return createBrowserClient(
    process.env.NEXT_PUBLIC_SUPABASE_URL!,
    process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY!,
  );
}
