import { createServerClient } from "@supabase/ssr";
import { NextResponse, type NextRequest } from "next/server";

import { requiresLogin } from "@/lib/auth/routes";

/**
 * セッションの更新と、未ログインの追い出し。
 *
 * **Next.js 16 で `middleware.ts` は `proxy.ts` に改名された。**
 *
 * ここが要るのは、**Server Component から Cookie を書けない**ため。
 * アクセストークンは1時間で切れるので、更新した Cookie を誰かが書かないと
 * 1時間でログアウトされる。それをここでやる。
 */
export default async function proxy(req: NextRequest) {
  let res = NextResponse.next({ request: req });

  const supabase = createServerClient(
    process.env.NEXT_PUBLIC_SUPABASE_URL!,
    process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY!,
    {
      cookies: {
        getAll: () => req.cookies.getAll(),
        setAll: (list) => {
          for (const { name, value } of list) req.cookies.set(name, value);
          res = NextResponse.next({ request: req });
          for (const { name, value, options } of list) res.cookies.set(name, value, options);
        },
      },
    },
  );

  // **getUser を呼ぶことがトークン更新の引き金になる。** 消さない
  const {
    data: { user },
  } = await supabase.auth.getUser();

  if (!user && requiresLogin(req.nextUrl.pathname)) {
    const to = req.nextUrl.clone();
    to.pathname = "/login";
    // 戻り先を持たせる。記録の途中で切れたときに元の画面へ帰す
    to.searchParams.set("next", req.nextUrl.pathname);

    return NextResponse.redirect(to);
  }

  // ログイン済みならログイン画面には用が無い
  if (user && req.nextUrl.pathname === "/login") {
    const to = req.nextUrl.clone();
    to.pathname = "/";
    to.search = "";

    return NextResponse.redirect(to);
  }

  return res;
}

export const config = {
  /*
   * 静的ファイルでは動かさない。画像やフォントのたびに Supabase へ
   * 問い合わせると、表示が目に見えて遅くなる
   */
  matcher: [
    "/((?!_next/static|_next/image|favicon.ico|sw.js|.*\\.(?:png|jpg|jpeg|svg|ico|webp)$).*)",
  ],
};
