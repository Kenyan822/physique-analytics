import Link from "next/link";

import { serverApi } from "@/lib/api/server";
import { tolerate } from "@/lib/api/tolerate";

import { MonthlyTargetsView } from "./MonthlyTargetsView";
import { PlanBlocksEditor } from "./PlanBlocksEditor";
import { loadMonthlyTargets, saveBlocks } from "./actions";

export const metadata = { title: "計画 | physique" };

export default async function PlanPage() {
  const api = serverApi();
  const [{ items: blocks }, targets] = await Promise.all([
    api.listPlanBlocks(),
    // **422 はエラーではなく「出せない理由」**（ブロック未登録・身長未設定など）。
    // ここで落とすとページごと開けなくなり、直しに行けない
    tolerate(() => serverApi().monthlyTargets("configured"), [422]),
  ]);

  return (
    <main className="mx-auto w-full max-w-md lg:max-w-4xl">
      <header className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-line bg-bg/95 px-4 py-3 backdrop-blur lg:px-6 lg:py-4">
        <h1 className="text-lg font-semibold lg:text-xl">3年計画</h1>
        <Link
          href="/"
          className="pressable rounded-full border border-line px-3 py-1.5 text-sm text-muted"
        >
          今日の記録
        </Link>
      </header>

      <div className="flex flex-col gap-4 px-4 py-4 lg:px-6">
        <PlanBlocksEditor blocks={blocks} saveBlocks={saveBlocks} />
        <MonthlyTargetsView
          initial={targets.value}
          initialMessage={targets.unavailable}
          loadMonthlyTargets={loadMonthlyTargets}
        />
      </div>
    </main>
  );
}
