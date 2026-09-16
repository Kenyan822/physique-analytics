import "server-only";

import { createServerClient } from "@supabase/ssr";
import { cookies } from "next/headers";

/**
 * サーバ側の Supabase クライアント。
 *
 * **Next.js 16 の `cookies()` は非同期**（`await cookies()`）。
 * Server Component からは Cookie を書けないので、セッションの更新は
 * `proxy.ts` が受け持つ。ここで set が呼ばれても黙って捨てる。
 */
export async function supabaseServer() {
  const store = await cookies();

  return createServerClient(url(), anonKey(), {
    cookies: {
      getAll: () => store.getAll(),
      setAll: (list) => {
        try {
          for (const { name, value, options } of list) store.set(name, value, options);
        } catch {
          // Server Component からの呼び出し。proxy.ts が更新するので無視してよい
        }
      },
    },
  });
}

/**
 * ログイン中ユーザーのアクセストークン（JWT）。未ログインなら null。
 *
 * **これを Go API に渡す。** API は `sub` を見て許可リストと突き合わせる
 * （ALLOWED_USER_IDS）。
 */
export async function accessToken(): Promise<string | null> {
  const supabase = await supabaseServer();
  const { data } = await supabase.auth.getSession();

  return data.session?.access_token ?? null;
}

/**
 * 検証済みのユーザー。未ログインなら null。
 *
 * **`getSession()` ではなくこちらで判定する。** getSession は Cookie の中身を
 * そのまま返すだけで、署名を検証しない。認可の判断に使ってはいけない。
 */
export async function currentUser() {
  const supabase = await supabaseServer();
  const { data } = await supabase.auth.getUser();

  return data.user;
}

function url(): string {
  const v = process.env.NEXT_PUBLIC_SUPABASE_URL;
  if (!v) throw new Error("NEXT_PUBLIC_SUPABASE_URL が設定されていない（.env.local を見る）");

  return v;
}

function anonKey(): string {
  const v = process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY;
  if (!v) throw new Error("NEXT_PUBLIC_SUPABASE_ANON_KEY が設定されていない（.env.local を見る）");

  return v;
}
