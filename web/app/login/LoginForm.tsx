"use client";

import { useState, useTransition } from "react";

import type { LoginResult } from "./actions";

type Props = {
  next: string;
  login: (email: string, password: string, next: string) => Promise<LoginResult>;
};

export function LoginForm({ next, login }: Props) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  function submit() {
    if (email.trim() === "" || password === "") return;
    setError(null);

    startTransition(async () => {
      // 成功すると redirect するのでここには戻ってこない
      const res = await login(email.trim(), password, next);
      setError(res.message);
    });
  }

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
      className="flex flex-col gap-3"
    >
      <label className="flex flex-col gap-1">
        <span className="text-xs text-muted">メールアドレス</span>
        <input
          type="email"
          autoComplete="email"
          required
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="h-12 rounded-xl border border-line bg-surface-2 px-3 text-base outline-none focus:border-accent"
        />
      </label>

      <label className="flex flex-col gap-1">
        <span className="text-xs text-muted">パスワード</span>
        <input
          type="password"
          autoComplete="current-password"
          required
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="h-12 rounded-xl border border-line bg-surface-2 px-3 text-base outline-none focus:border-accent"
        />
      </label>

      <button
        type="submit"
        disabled={pending}
        className="pressable mt-2 h-12 rounded-xl bg-accent text-base font-bold text-accent-ink disabled:opacity-40"
      >
        {pending ? "確認中…" : "ログイン"}
      </button>

      {error && (
        <p
          role="alert"
          className="rounded-xl border border-danger/40 bg-danger/10 px-3 py-2 text-sm text-danger"
        >
          {error}
        </p>
      )}
    </form>
  );
}
