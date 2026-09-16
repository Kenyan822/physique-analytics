import Link from "next/link";

import { ExportPanel } from "./ExportPanel";
import { ImportPanel } from "./ImportPanel";
import { PlanBackupPanel } from "./PlanBackupPanel";
import { exportCsv, exportPlan, importCsv, importPlan } from "./actions";

export const metadata = { title: "CSV | physique" };

export default function TransferPage() {
  return (
    <main className="mx-auto w-full max-w-md lg:max-w-4xl">
      <header className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-line bg-bg/95 px-4 py-3 backdrop-blur lg:px-6 lg:py-4">
        <h1 className="text-lg font-semibold lg:text-xl">CSV</h1>
        <Link
          href="/"
          className="pressable rounded-full border border-line px-3 py-1.5 text-sm text-muted"
        >
          今日の記録
        </Link>
      </header>

      <div className="flex flex-col gap-4 px-4 py-4 lg:grid lg:grid-cols-2 lg:items-start lg:gap-6 lg:px-6">
        <ImportPanel importCsv={importCsv} />
        <ExportPanel exportCsv={exportCsv} />
        <div className="lg:col-span-2">
          <PlanBackupPanel exportPlan={exportPlan} importPlan={importPlan} />
        </div>
      </div>
    </main>
  );
}
