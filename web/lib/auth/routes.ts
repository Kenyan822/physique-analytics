/** ログインしていなくても開けるパス。 */
export const PUBLIC_PATHS = ["/login", "/auth"];

/**
 * そのパスがログイン必須か。
 *
 * **静的ファイルと Next の内部パスは対象外にする。** ここで弾かないと、
 * CSS や画像の取得までリダイレクトされて画面が壊れる。
 */
export function requiresLogin(pathname: string): boolean {
  if (pathname.startsWith("/_next") || pathname === "/favicon.ico") return false;
  // 拡張子があるものは静的ファイル（/sw.js など）
  if (/\.[a-z0-9]+$/i.test(pathname)) return false;

  return !PUBLIC_PATHS.some((p) => pathname === p || pathname.startsWith(`${p}/`));
}
