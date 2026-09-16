import { SiteHeader } from "@/app/SiteHeader";
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
    <>
      <SiteHeader title="3年計画" />

      <main className="mx-auto w-full max-w-6xl">
        <div className="flex flex-col gap-4 px-4 py-4 lg:px-6">
          <PlanBlocksEditor blocks={blocks} saveBlocks={saveBlocks} />
          <MonthlyTargetsView
            initial={targets.value}
            initialMessage={targets.unavailable}
            loadMonthlyTargets={loadMonthlyTargets}
          />
        </div>
      </main>
    </>
  );
}
