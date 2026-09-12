import Link from "next/link";

import { serverApi } from "@/lib/api/server";
import { formatJstDate, todayJst } from "@/lib/jst";

import { BodyForm } from "./BodyForm";
import { saveDaily, saveMeasurement } from "./actions";

export const metadata = { title: "体組成 | physique" };

/** 前回の体重を探す範囲。測り忘れが続いても拾えるだけ遡る */
const WEIGHT_LOOKBACK_DAYS = 30;

export default async function BodyPage() {
  const date = todayJst();
  const api = serverApi();

  const from = shiftDays(date, -WEIGHT_LOOKBACK_DAYS);
  const [{ items: daily }, { items: measurements }] = await Promise.all([
    api.listDailyMetrics({ from, to: date }),
    // 周囲長は月1回程度なので期間で絞らない。今日より後は比較対象にしない
    api.listMeasurements({ to: date }),
  ]);

  const today = daily.find((d) => d.date === date) ?? null;
  // 前回値は「今日より前で、その項目が入っている直近」。
  // 一覧は新しい順に返るので先頭から探せばよい。体脂肪率は測れた日だけ
  // 入るので、体重とは別の日が前回になる
  const before = daily.filter((d) => d.date !== date);
  const lastWeight = before.find((d) => d.weightKg != null) ?? null;
  const lastBodyfat = before.find((d) => d.bodyfatPct != null) ?? null;

  // 今日の分と、その前の分を分けて渡す。**今日すでに測っていても
  // 前回との差は出す**（要件 B-03）。一緒くたにすると、測り直した日だけ
  // 差分が消えて「変わっていない」ように見える
  const todayMeasurement = measurements.find((m) => m.date === date) ?? null;
  const previousMeasurement = measurements.find((m) => m.date !== date) ?? null;

  return (
    <main className="mx-auto w-full max-w-md lg:max-w-5xl">
      <header className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-line bg-bg/95 px-4 py-3 backdrop-blur lg:px-6 lg:py-4">
        <h1 className="text-lg font-semibold lg:text-xl">体組成 {formatJstDate(date)}</h1>
        <Link
          href="/"
          className="pressable rounded-full border border-line px-3 py-1.5 text-sm text-muted"
        >
          今日の記録
        </Link>
      </header>

      <div className="pt-4">
        <BodyForm
          date={date}
          today={today}
          todayMeasurement={todayMeasurement}
          previousMeasurement={previousMeasurement}
          lastWeightKg={lastWeight?.weightKg ?? null}
          lastBodyfatPct={lastBodyfat?.bodyfatPct ?? null}
          saveDaily={saveDaily}
          saveMeasurement={saveMeasurement}
        />
      </div>
    </main>
  );
}

/** YYYY-MM-DD を n 日ずらす。JST 固定なので UTC で計算してよい（ADR-0013） */
function shiftDays(date: string, n: number): string {
  const [y, m, d] = date.split("-").map(Number);
  const t = new Date(Date.UTC(y, m - 1, d + n));

  return t.toISOString().slice(0, 10);
}
