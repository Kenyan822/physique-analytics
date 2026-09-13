/**
 * ログイン後の戻り先を安全にする。
 *
 * **自分のサイト内のパスしか受け付けない。** `next=https://evil.example`
 * を素通しすると、ログイン直後に外部サイトへ飛ばせてしまう（open redirect）。
 * `//evil.example` もプロトコル相対URLとして外部に飛ぶので弾く。
 */
export function safeNext(next: string | null | undefined): string {
  if (!next) return "/";

  return next.startsWith("/") && !next.startsWith("//") ? next : "/";
}
