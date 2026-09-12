import Link from "next/link";

import { serverApi } from "@/lib/api/server";
import { todayJst } from "@/lib/jst";

import { ContestEditor } from "./ContestEditor";
import { SettingsForm } from "./SettingsForm";
import { TemplateEditor } from "./TemplateEditor";
import { savePlan } from "./actions";
import { createContest, deleteContest } from "./contestActions";
import { createTemplate, deleteTemplate, updateTemplate } from "./templateActions";

export const metadata = { title: "設定 | physique" };

export default async function SettingsPage() {
  const api = serverApi();
  const today = todayJst();
  const [plan, { items: templates }, { items: exercises }, { items: contests }] = await Promise.all(
    [api.getPlan(), api.listTemplates(), api.listExercises({}), api.listContests()],
  );

  return (
    <main className="mx-auto w-full max-w-md lg:max-w-3xl">
      <header className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-line bg-bg/95 px-4 py-3 backdrop-blur lg:px-6 lg:py-4">
        <h1 className="text-lg font-semibold lg:text-xl">設定</h1>
        <Link
          href="/"
          className="pressable rounded-full border border-line px-3 py-1.5 text-sm text-muted"
        >
          今日の記録
        </Link>
      </header>

      <div className="pt-4">
        <div className="flex flex-col gap-4 px-4 pb-4 lg:px-6">
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
        <SettingsForm plan={plan} today={today} savePlan={savePlan} />
      </div>
    </main>
  );
}
