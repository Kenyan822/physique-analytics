"use client";

import { useTransition } from "react";

import { logout } from "./login/actions";

export function LogoutButton() {
  const [pending, startTransition] = useTransition();

  return (
    <button
      type="button"
      onClick={() => startTransition(() => logout())}
      disabled={pending}
      className="pressable shrink-0 rounded-full border border-line bg-surface px-3.5 py-1.5 text-sm text-muted disabled:opacity-40"
    >
      {pending ? "…" : "ログアウト"}
    </button>
  );
}
