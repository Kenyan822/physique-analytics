import Link from "next/link";

import { serverApi } from "@/lib/api/server";
import { todayJst } from "@/lib/jst";

import { BloodView } from "./BloodView";
import { createBloodTest, deleteBloodTest } from "./actions";

export const metadata = { title: "血液検査 | physique" };

export default async function BloodPage() {
  const { items } = await serverApi().listBloodTests();

  return (
    <main className="mx-auto w-full max-w-md lg:max-w-5xl">
      <header className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-line bg-bg/95 px-4 py-3 backdrop-blur lg:px-6 lg:py-4">
        <h1 className="text-lg font-semibold lg:text-xl">血液検査</h1>
        <Link
          href="/"
          className="pressable rounded-full border border-line px-3 py-1.5 text-sm text-muted"
        >
          今日の記録
        </Link>
      </header>

      <div className="pt-4">
        <BloodView
          tests={items}
          today={todayJst()}
          createBloodTest={createBloodTest}
          deleteBloodTest={deleteBloodTest}
        />
      </div>
    </main>
  );
}
