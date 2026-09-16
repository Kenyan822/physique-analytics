import { SiteHeader } from "@/app/SiteHeader";
import { serverApi } from "@/lib/api/server";
import { todayJst } from "@/lib/jst";

import { BloodView } from "./BloodView";
import { createBloodTest, deleteBloodTest } from "./actions";

export const metadata = { title: "血液検査 | physique" };

export default async function BloodPage() {
  const { items } = await serverApi().listBloodTests();

  return (
    <>
      <SiteHeader title="血液検査" />

      <main className="mx-auto w-full max-w-6xl">
        <div className="pt-4">
          <BloodView
            tests={items}
            today={todayJst()}
            createBloodTest={createBloodTest}
            deleteBloodTest={deleteBloodTest}
          />
        </div>
      </main>
    </>
  );
}
