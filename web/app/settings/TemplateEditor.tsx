"use client";

import { useState, useTransition } from "react";

import type { Exercise, Template, TemplateInput, TemplateItem } from "@/lib/api/client";

import type { TemplateResult } from "./templateActions";

type Props = {
  templates: Template[];
  exercises: Exercise[];
  createTemplate: (input: TemplateInput) => Promise<TemplateResult>;
  updateTemplate: (id: string, input: TemplateInput) => Promise<TemplateResult>;
  deleteTemplate: (id: string) => Promise<{ ok: boolean; message?: string }>;
};

/** 1日の種目数の上限（要件 T-05 の前提）。超えると回復が追いつかない */
const MAX_ITEMS = 8;

/**
 * トレーニングテンプレートの管理（要件 P-06）。
 *
 * 記録画面（/log）では**呼び出すだけ**で、ここで作る。
 * ジムで構成を編集することはないので、入力の速さより一覧性を優先する。
 */
export function TemplateEditor({
  templates,
  exercises,
  createTemplate,
  updateTemplate,
  deleteTemplate,
}: Props) {
  const [list, setList] = useState<Template[]>(templates);
  const [editing, setEditing] = useState<{
    id: string | null;
    name: string;
    items: TemplateItem[];
  } | null>(null);
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [pending, startTransition] = useTransition();

  const nameById = new Map(exercises.map((e) => [e.id, e.name]));

  function startNew() {
    setMessage(null);
    setEditing({ id: null, name: "", items: [] });
  }

  function startEdit(t: Template) {
    setMessage(null);
    setEditing({ id: t.id, name: t.name, items: t.items.map((i) => ({ ...i })) });
  }

  // 種目が無いテンプレートは呼び出しても何も起きない。サーバも 400 で弾く
  const canSave = editing !== null && editing.name.trim() !== "" && editing.items.length > 0;

  function save() {
    if (!canSave || editing === null) return;

    // order は配列の並びで振り直す。手で管理させると必ずずれる
    const input: TemplateInput = {
      name: editing.name.trim(),
      items: editing.items.map((it, i) => ({ ...it, order: i + 1 })),
    };

    startTransition(async () => {
      const res = editing.id
        ? await updateTemplate(editing.id, input)
        : await createTemplate(input);

      if (!res.ok) {
        setMessage({ ok: false, text: res.message });

        return;
      }
      setList((prev) =>
        editing.id
          ? prev.map((t) => (t.id === editing.id ? res.template : t))
          : [...prev, res.template],
      );
      setEditing(null);
      setMessage({ ok: true, text: "保存した" });
    });
  }

  function remove(id: string) {
    startTransition(async () => {
      const res = await deleteTemplate(id);
      if (!res.ok) {
        setMessage({ ok: false, text: res.message ?? "削除できなかった" });

        return;
      }
      setList((prev) => prev.filter((t) => t.id !== id));
      if (editing?.id === id) setEditing(null);
    });
  }

  return (
    <section className="rounded-2xl border border-line bg-surface p-4">
      <div className="mb-3 flex items-baseline justify-between">
        <h2 className="text-sm font-medium">トレーニングテンプレート</h2>
        <span className="text-[11px] text-muted">記録画面から呼び出す</span>
      </div>

      <ul className="flex flex-col gap-2">
        {list.map((t) => (
          <li
            key={t.id}
            className="flex items-center gap-2 rounded-xl border border-line bg-surface-2 p-3"
          >
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm">{t.name}</span>
              <span className="block truncate text-xs text-muted">
                {t.items.length === 0
                  ? "種目なし"
                  : t.items
                      .map((i) => `${nameById.get(i.exerciseId) ?? "?"}×${i.targetSets}`)
                      .join(" / ")}
              </span>
            </span>
            <button
              type="button"
              onClick={() => startEdit(t)}
              disabled={pending}
              className="pressable shrink-0 rounded-lg border border-line px-2.5 py-1.5 text-xs text-muted disabled:opacity-40"
            >
              編集
            </button>
            <button
              type="button"
              aria-label={`${t.name}を削除`}
              onClick={() => remove(t.id)}
              disabled={pending}
              className="pressable shrink-0 rounded-lg px-2 text-muted disabled:opacity-40"
            >
              ×
            </button>
          </li>
        ))}
      </ul>

      {editing === null ? (
        <button
          type="button"
          onClick={startNew}
          className="pressable mt-3 h-10 w-full rounded-xl border border-line text-sm text-muted"
        >
          テンプレートを追加
        </button>
      ) : (
        <div className="mt-3 rounded-xl border border-accent/40 bg-surface-2 p-3">
          <input
            type="text"
            aria-label="テンプレート名"
            value={editing.name}
            placeholder="Day1 胸"
            onChange={(e) => setEditing({ ...editing, name: e.target.value })}
            className="h-10 w-full rounded-lg border border-line bg-surface px-2 text-sm outline-none focus:border-accent"
          />

          <ul className="mt-2 flex flex-col gap-2">
            {editing.items.map((it, i) => (
              <li key={i} className="flex flex-wrap items-center gap-2">
                <select
                  aria-label={`${i + 1}種目目`}
                  value={it.exerciseId}
                  onChange={(e) =>
                    setEditing({
                      ...editing,
                      items: editing.items.map((x, j) =>
                        j === i ? { ...x, exerciseId: e.target.value } : x,
                      ),
                    })
                  }
                  className="h-10 min-w-0 flex-1 rounded-lg border border-line bg-surface px-2 text-sm outline-none focus:border-accent"
                >
                  {exercises.map((e) => (
                    <option key={e.id} value={e.id}>
                      {e.name}
                    </option>
                  ))}
                </select>
                <label className="flex shrink-0 items-center gap-1">
                  <input
                    type="text"
                    inputMode="numeric"
                    aria-label={`${i + 1}種目目のセット数`}
                    value={it.targetSets}
                    onChange={(e) =>
                      setEditing({
                        ...editing,
                        items: editing.items.map((x, j) =>
                          j === i
                            ? {
                                ...x,
                                targetSets: Math.max(1, Math.round(Number(e.target.value) || 1)),
                              }
                            : x,
                        ),
                      })
                    }
                    className="tnum h-10 w-12 rounded-lg border border-line bg-surface px-1 text-center text-sm outline-none focus:border-accent"
                  />
                  <span className="text-xs text-muted">セット</span>
                </label>
                <button
                  type="button"
                  aria-label={`${i + 1}種目目を外す`}
                  onClick={() =>
                    setEditing({ ...editing, items: editing.items.filter((_, j) => j !== i) })
                  }
                  className="pressable shrink-0 rounded-lg px-2 text-muted"
                >
                  ×
                </button>
              </li>
            ))}
          </ul>

          {editing.items.length < MAX_ITEMS && exercises.length > 0 && (
            <button
              type="button"
              onClick={() =>
                setEditing({
                  ...editing,
                  items: [
                    ...editing.items,
                    {
                      exerciseId: exercises[0].id,
                      order: editing.items.length + 1,
                      targetSets: 3,
                    },
                  ],
                })
              }
              className="pressable mt-2 h-9 w-full rounded-lg border border-line text-xs text-muted"
            >
              種目を追加
            </button>
          )}

          {editing.items.length === 0 && (
            <p className="mt-2 text-xs text-muted">種目を1つ以上入れると保存できる</p>
          )}

          <div className="mt-3 flex gap-2">
            <button
              type="button"
              onClick={save}
              disabled={pending || !canSave}
              className="pressable h-10 flex-1 rounded-xl bg-accent text-sm font-bold text-accent-ink disabled:opacity-40"
            >
              {pending ? "保存中…" : "保存"}
            </button>
            <button
              type="button"
              onClick={() => setEditing(null)}
              className="pressable h-10 rounded-xl border border-line px-4 text-sm text-muted"
            >
              やめる
            </button>
          </div>
        </div>
      )}

      {message && (
        <p
          role="status"
          className={`mt-3 rounded-xl border px-3 py-2 text-sm ${
            message.ok
              ? "border-accent/40 bg-accent/10 text-accent"
              : "border-danger/40 bg-danger/10 text-danger"
          }`}
        >
          {message.text}
        </p>
      )}
    </section>
  );
}
