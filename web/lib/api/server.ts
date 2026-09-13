import "server-only";

import { accessToken } from "@/lib/supabase/server";

import { createClient, type ApiClient } from "./client";

/**
 * サーバ側から使う API クライアント。
 *
 * **`server-only` を付けているので Client Component から import するとビルドが落ちる。**
 * トークンを持つクライアントがブラウザに落ちるのを型ではなくビルドで防ぐ。
 *
 * **トークンは固定値ではなく、ログイン中ユーザーのもの**（#154）。
 * API 側は JWT の `sub` を許可リスト（ALLOWED_USER_IDS）と突き合わせる。
 *
 * トークンを**関数で渡している**のは、Cookie の読み取りが非同期なため。
 * ここで await すると `serverApi()` 自体が非同期になり、34箇所の呼び出しが
 * すべて変わる。呼び出しごとに解決する方が変更が小さい。
 */
export function serverApi(): ApiClient {
  const baseUrl = process.env.API_BASE_URL;
  if (!baseUrl) {
    // 起動時ではなく最初の呼び出しで落ちる。原因が分かるメッセージにする
    throw new Error("API_BASE_URL が設定されていない（.env.local を見る）");
  }

  // 未ログインなら Authorization を付けずに呼ぶ。API が 401 を返す
  return createClient({ baseUrl, token: async () => (await accessToken()) ?? undefined });
}
