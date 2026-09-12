import Link from "next/link";

import { serverApi } from "@/lib/api/server";
import { formatJstDate, todayJst } from "@/lib/jst";

export default async function Home() {
  const date = todayJst();
  const api = serverApi();

  const [{ items: sessions }, { items: exercises }] = await Promise.all([
    api.listWorkoutSessions({ from: date, to: date, limit: 1 }),
    api.listExercises({}),
  ]);

  const today = sessions[0];
  const nameById = new Map(exercises.map((e) => [e.id, e.name]));
  const groupById = new Map(exercises.map((e) => [e.id, e.muscleGroup]));

  // 種目ごとにまとめる。セットは setNo 順に並んでいる
  const byExercise = new Map<string, NonNullable<typeof today>["sets"]>();
  for (const s of today?.sets ?? []) {
    const list = byExercise.get(s.exerciseId) ?? [];
    list.push(s);
    byExercise.set(s.exerciseId, list);
  }

  const sets = today?.sets ?? [];
  const tonnage = sets.reduce((sum, s) => sum + s.weightKg * s.reps, 0);

  return (
    <main className="mx-auto w-full max-w-md md:max-w-3xl xl:max-w-5xl">
      <header className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-line bg-bg/95 px-4 py-3 backdrop-blur md:px-6 md:py-4">
        <h1 className="text-lg font-semibold md:text-xl">{formatJstDate(date)}</h1>
        <Link
          href="/log"
          className="pressable shrink-0 rounded-full bg-accent px-4 py-2 text-sm font-bold text-accent-ink"
        >
          記録する
        </Link>
      </header>

      {/*
       * 画面が増えたのでヘッダーから出した。狭い画面では横に流す。
       * ヘッダーに並べ続けると、一番押す「記録する」が潰れる
       */}
      <nav className="flex gap-2 overflow-x-auto border-b border-line px-4 py-2.5 md:px-6">
        <NavLink href="/meals">食事</NavLink>
        <NavLink href="/body">体組成</NavLink>
        <NavLink href="/plan">計画</NavLink>
        <NavLink href="/settings">設定</NavLink>
      </nav>

      <div className="flex flex-col gap-4 p-4 md:gap-6 md:p-6">
        {byExercise.size === 0 ? (
          <div className="rounded-2xl border border-line bg-surface p-8 text-center">
            <p className="text-sm text-muted">今日はまだ記録が無い</p>
            <Link
              href="/log"
              className="pressable mt-4 inline-block rounded-xl bg-accent px-5 py-3 text-sm font-bold text-accent-ink"
            >
              記録を始める
            </Link>
          </div>
        ) : (
          <>
            <div className="grid grid-cols-3 gap-2 md:gap-4">
              <Stat label="種目" value={String(byExercise.size)} />
              <Stat label="セット" value={String(sets.length)} />
              <Stat label="トン数" value={Math.round(tonnage).toLocaleString()} unit="kg" />
            </div>

            <ul className="grid gap-3 md:grid-cols-2 md:gap-4 xl:grid-cols-3">
              {[...byExercise].map(([exerciseId, list]) => (
                <li key={exerciseId} className="rounded-2xl border border-line bg-surface p-4">
                  <div className="mb-2 flex items-baseline justify-between gap-2">
                    <h2 className="truncate font-semibold">
                      {nameById.get(exerciseId) ?? "（不明な種目）"}
                    </h2>
                    <span className="shrink-0 text-xs text-muted">{groupById.get(exerciseId)}</span>
                  </div>
                  <ul className="tnum flex flex-wrap gap-2 text-sm">
                    {list.map((s) => (
                      <li key={s.id} className="rounded-lg bg-surface-2 px-2 py-1">
                        {s.weightKg}×{s.reps}
                        {s.rir != null && <span className="text-muted">@{s.rir}</span>}
                      </li>
                    ))}
                  </ul>
                </li>
              ))}
            </ul>
          </>
        )}
      </div>
    </main>
  );
}

function NavLink({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <Link
      href={href}
      className="pressable shrink-0 rounded-full border border-line bg-surface px-3.5 py-1.5 text-sm text-muted"
    >
      {children}
    </Link>
  );
}

function Stat({ label, value, unit }: { label: string; value: string; unit?: string }) {
  return (
    <div className="rounded-2xl border border-line bg-surface px-3 py-3 text-center">
      <div className="text-[11px] text-muted">{label}</div>
      <div className="tnum text-2xl font-semibold leading-tight">
        {value}
        {unit && <span className="ml-0.5 text-sm font-normal text-muted">{unit}</span>}
      </div>
    </div>
  );
}
