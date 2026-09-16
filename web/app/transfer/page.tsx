import { SiteHeader } from "@/app/SiteHeader";
import { ExportPanel } from "./ExportPanel";
import { ImportPanel } from "./ImportPanel";
import { PlanBackupPanel } from "./PlanBackupPanel";
import { exportCsv, exportPlan, importCsv, importPlan } from "./actions";

export const metadata = { title: "CSV | physique" };

export default function TransferPage() {
  return (
    <>
      <SiteHeader title="データ" />

      <main className="mx-auto w-full max-w-6xl">
        <div className="flex flex-col gap-4 px-4 py-4 lg:grid lg:grid-cols-2 lg:items-start lg:gap-6 lg:px-6">
          <ImportPanel importCsv={importCsv} />
          <ExportPanel exportCsv={exportCsv} />
          <div className="lg:col-span-2">
            <PlanBackupPanel exportPlan={exportPlan} importPlan={importPlan} />
          </div>
        </div>
      </main>
    </>
  );
}
