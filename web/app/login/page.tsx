import { safeNext } from "@/lib/auth/next";

import { LoginForm } from "./LoginForm";
import { login } from "./actions";

export const metadata = { title: "ログイン | physique" };

export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<{ next?: string }>;
}) {
  // Next.js 16 では searchParams が非同期
  const { next } = await searchParams;

  return (
    <main className="mx-auto flex w-full max-w-sm flex-1 flex-col justify-center px-4 py-12">
      <h1 className="mb-1 text-xl font-semibold">physique</h1>
      <p className="mb-6 text-sm text-muted">記録を見るにはログインが要る</p>

      <LoginForm next={safeNext(next)} login={login} />
    </main>
  );
}
