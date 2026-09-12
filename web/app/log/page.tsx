import Link from "next/link";

import { serverApi } from "@/lib/api/server";
import { formatJstDate, todayJst } from "@/lib/jst";

import { LogForm } from "./LogForm";
import { loadLastPerformance, recordSet } from "./actions";

export const metadata = { title: "記録 | physique-analytics" };

export default async function LogPage() {
  const date = todayJst();
  const { items: exercises } = await serverApi().listExercises({});

  return (
    <main className="mx-auto flex max-w-md flex-col gap-6 p-4">
      <header className="flex items-baseline justify-between">
        <h1 className="text-xl font-semibold">記録 {formatJstDate(date)}</h1>
        <Link href="/" className="text-sm text-gray-600 underline dark:text-gray-400">
          今日の記録
        </Link>
      </header>

      <LogForm
        exercises={exercises}
        loadLast={loadLastPerformance}
        recordSet={recordSet}
        date={date}
      />
    </main>
  );
}
