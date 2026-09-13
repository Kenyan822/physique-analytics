"use client";

import { useState, useTransition } from "react";

import type { BloodTest, BloodTestItem } from "@/lib/api/client";

type Props = {
  tests: BloodTest[];
  onDeleted: (id: string) => void;
  deleteBloodTest: (id: string) => Promise<{ ok: boolean; message?: string }>;
};

/**
 * 過去の検査（要件 B-08）。
 *
 * **まず基準外の件数を見る。** 項目は数十行あるので、全部読ませない。
 * 開いたときの並びは検査票のまま（サーバが順序を保って返す）。
 */
export function BloodTestList({ tests, onDeleted, deleteBloodTest }: Props) {
  const [openId, setOpenId] = useState<string | null>(tests[0]?.id ?? null);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  function remove(t: BloodTest) {
    setError(null);
    startTransition(async () => {
      const res = await deleteBloodTest(t.id);
      if (!res.ok) {
        setError(res.message ?? "削除できなかった");

        return;
      }
      onDeleted(t.id);
    });
  }

  if (tests.length === 0) {
    return (
      <section className="rounded-2xl border border-line bg-surface p-8 text-center">
        <p className="text-sm text-muted">まだ記録が無い。検査票を見ながら下から入れる</p>
      </section>
    );
  }

  return (
    <section className="flex flex-col gap-3">
      {error && (
        <p
          role="alert"
          className="rounded-xl border border-danger/40 bg-danger/10 px-3 py-2 text-sm text-danger"
        >
          {error}
        </p>
      )}

      {tests.map((t) => {
        const open = openId === t.id;
        const out = t.outOfRangeCount ?? 0;

        return (
          <article key={t.id} className="rounded-2xl border border-line bg-surface">
            <div className="flex items-center gap-2 p-4">
              <button
                type="button"
                onClick={() => setOpenId(open ? null : t.id)}
                aria-expanded={open}
                className="pressable flex min-w-0 flex-1 items-center gap-3 text-left"
              >
                <span className="min-w-0 flex-1">
                  <span className="tnum block text-base font-semibold">{t.date}</span>
                  <span className="block truncate text-xs text-muted">
                    {t.clinic ?? "クリニック未記入"} / {t.items.length} 項目
                  </span>
                </span>
                {out > 0 ? (
                  <span className="tnum shrink-0 rounded-full bg-warn/15 px-2.5 py-1 text-xs font-semibold text-warn">
                    基準外 {out}
                  </span>
                ) : (
                  <span className="shrink-0 text-xs text-muted">基準内</span>
                )}
                <span className="shrink-0 text-muted" aria-hidden>
                  {open ? "▾" : "›"}
                </span>
              </button>
              <button
                type="button"
                aria-label={`${t.date}の検査を削除`}
                onClick={() => remove(t)}
                disabled={pending}
                className="pressable shrink-0 rounded-lg px-2 py-1 text-muted disabled:opacity-30"
              >
                ×
              </button>
            </div>

            {open && (
              <div className="border-t border-line px-4 pb-4 pt-3">
                {t.note && <p className="mb-3 text-xs text-muted">{t.note}</p>}
                <ul className="flex flex-col">
                  {t.items.map((item, i) => (
                    <ItemRow key={`${item.name}-${i}`} item={item} />
                  ))}
                </ul>
              </div>
            )}
          </article>
        );
      })}
    </section>
  );
}

/**
 * 1項目。**基準が無い項目は判定しない**（要件 B-08）。
 * 検査票に書いていない基準を当てて「異常」と言わない。
 */
function ItemRow({ item }: { item: BloodTestItem }) {
  const tone = item.flag === "high" || item.flag === "low" ? "text-warn font-semibold" : "text-ink";

  return (
    <li className="flex items-baseline justify-between gap-3 border-b border-line/50 py-1.5 text-sm last:border-b-0">
      <span className="min-w-0 truncate text-muted">{item.name}</span>
      <span className="flex shrink-0 items-baseline gap-1.5">
        <span className={`tnum ${tone}`}>
          {item.value != null ? item.value : (item.textValue ?? "—")}
        </span>
        {item.unit && <span className="text-[11px] text-muted">{item.unit}</span>}
        <Range item={item} />
      </span>
    </li>
  );
}

function Range({ item }: { item: BloodTestItem }) {
  if (item.refLow == null && item.refHigh == null) {
    return <span className="w-24 shrink-0 text-right text-[11px] text-muted">基準なし</span>;
  }

  return (
    <span className="tnum w-24 shrink-0 text-right text-[11px] text-muted">
      {item.refLow ?? ""}–{item.refHigh ?? ""}
    </span>
  );
}
