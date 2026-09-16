import { ApiError } from "./client";

export type Tolerated<T> = {
  /** 取れた値。想定内の失敗なら null */
  value: T | null;
  /** 出せない理由。取れていれば null */
  unavailable: string | null;
};

/**
 * 「まだ出せない」をエラーではなく値として扱う。
 *
 * **画面を落とさないために要る。** フェーズ未登録（422）や写真の保存先未設定（503）は
 * 異常ではなく設定が済んでいないだけで、ここで throw すると**ページごと開けなくなり、
 * 設定を直しに行けない**。
 *
 * **想定したステータスだけを飲み込む。** 500 まで握りつぶすと、本当の障害が
 * 画面上は「未設定」に見えて原因を見誤る。
 */
export async function tolerate<T>(
  load: () => Promise<T>,
  statuses: number[],
): Promise<Tolerated<T>> {
  try {
    return { value: await load(), unavailable: null };
  } catch (e) {
    if (e instanceof ApiError && statuses.includes(e.status)) {
      return { value: null, unavailable: reason(e) };
    }

    throw e;
  }
}

function reason(e: ApiError): string {
  const field = e.problem?.errors?.[0];

  return field?.message ?? e.problem?.detail ?? e.problem?.title ?? e.message;
}
