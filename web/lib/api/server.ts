import "server-only";

import { createClient, type ApiClient } from "./client";

/**
 * サーバ側から使う API クライアント。
 *
 * **`server-only` を付けているので Client Component から import するとビルドが落ちる。**
 * トークンを持つクライアントがブラウザに落ちるのを型ではなくビルドで防ぐ。
 */
export function serverApi(): ApiClient {
  const baseUrl = process.env.API_BASE_URL;
  if (!baseUrl) {
    // 起動時ではなく最初の呼び出しで落ちる。原因が分かるメッセージにする
    throw new Error("API_BASE_URL が設定されていない（.env.local を見る）");
  }

  // Phase 1a の Web はサーバ側でだけ API を叩く。
  // Supabase のログインを入れるまでは、API 側を AUTH_DISABLED=true で動かすか
  // API_TOKEN に検証済みのトークンを入れる
  return createClient({ baseUrl, token: process.env.API_TOKEN });
}
