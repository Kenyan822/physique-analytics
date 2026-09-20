import { SiteHeader } from "@/app/SiteHeader";
import { serverApi } from "@/lib/api/server";
import { todayJst } from "@/lib/jst";

import { ContestEditor } from "./ContestEditor";
import { SettingsForm } from "./SettingsForm";
import { TargetEditor } from "./TargetEditor";
import { TemplateEditor } from "./TemplateEditor";
import { savePlan } from "./actions";
import { createContest, deleteContest } from "./contestActions";
import { clearManualTargets, saveManualTargets } from "./targetActions";
import { createTemplate, deleteTemplate, updateTemplate } from "./templateActions";

export const metadata = { title: "設定 | physique" };

export default async function SettingsPage() {
  const api = serverApi();
  const today = todayJst();
  const [plan, { items: templates }, { items: exercises }, { items: contests }, { targets }] =
    await Promise.all([
      api.getPlan(),
      api.listTemplates(),
      api.listExercises({}),
      api.listContests(),
      api.manualTargets(),
    ]);

  return (
    <>
      <SiteHeader title="設定" />

      <main className="mx-auto w-full max-w-6xl">
        {/*
         * 画面が広いときは2列にする。1列のまま伸ばすと、入力欄が横に間延びして
         * どこを見ればいいか分からなくなる
         */}
        <div className="pt-4 lg:grid lg:grid-cols-2 lg:items-start lg:gap-6 lg:px-6">
          <div className="flex flex-col gap-4 px-4 pb-4 lg:px-0">
            <TemplateEditor
              templates={templates}
              exercises={exercises}
              createTemplate={createTemplate}
              updateTemplate={updateTemplate}
              deleteTemplate={deleteTemplate}
            />
            <ContestEditor
              contests={contests}
              today={today}
              createContest={createContest}
              deleteContest={deleteContest}
            />
          </div>
          <div className="flex flex-col gap-4 px-4 pb-4 lg:px-0">
            {/* 毎日見る数字なので、計画の設定より前に置く */}
            <TargetEditor
              current={targets}
              save={saveManualTargets}
              clear={clearManualTargets}
            />
            <SettingsForm plan={plan} today={today} savePlan={savePlan} />
          </div>
        </div>
      </main>
    </>
  );
}
