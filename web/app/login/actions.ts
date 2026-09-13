"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";

import { safeNext } from "@/lib/auth/next";
import { supabaseServer } from "@/lib/supabase/server";

export type LoginResult = { ok: false; message: string };

/**
 * メールとパスワードでログインする。
 *
 * 成功したら redirect するので、この関数は「失敗したときだけ返る」。
 */
export async function login(email: string, password: string, next: string): Promise<LoginResult> {
  const supabase = await supabaseServer();
  const { error } = await supabase.auth.signInWithPassword({ email, password });

  if (error) {
    // **理由を細かく出さない。** 「メールが存在しない」と「パスワードが違う」を
    // 区別して返すと、登録済みのメールを総当たりで探れる
    return { ok: false, message: "メールアドレスかパスワードが違う" };
  }

  revalidatePath("/", "layout");
  redirect(safeNext(next));
}

/** ログアウトする。 */
export async function logout(): Promise<void> {
  const supabase = await supabaseServer();
  await supabase.auth.signOut();
  revalidatePath("/", "layout");
  redirect("/login");
}
