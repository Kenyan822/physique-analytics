"use client";

import { useState } from "react";

import type { BloodTest, BloodTestInput } from "@/lib/api/client";

import { BloodTestForm } from "./BloodTestForm";
import { BloodTestList } from "./BloodTestList";
import type { BloodTestResult } from "./actions";

type Props = {
  tests: BloodTest[];
  today: string;
  createBloodTest: (input: BloodTestInput) => Promise<BloodTestResult>;
  deleteBloodTest: (id: string) => Promise<{ ok: boolean; message?: string }>;
};

/**
 * 血液検査（要件 B-08）。
 *
 * 一覧と入力で同じ配列を見る。登録した検査がその場で一覧に出ないと、
 * 入ったかどうかが分からない。
 */
export function BloodView({ tests, today, createBloodTest, deleteBloodTest }: Props) {
  const [list, setList] = useState<BloodTest[]>(tests);

  return (
    /* 画面が広いときは一覧と入力を横に並べる。検査票を見ながら打つ */
    <div className="flex flex-col gap-4 px-4 pb-8 lg:grid lg:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)] lg:items-start lg:gap-6 lg:px-6">
      <BloodTestList
        tests={list}
        onDeleted={(id) => setList((prev) => prev.filter((t) => t.id !== id))}
        deleteBloodTest={deleteBloodTest}
      />
      <BloodTestForm
        today={today}
        createBloodTest={createBloodTest}
        // 新しい順に並べる。サーバも同じ順で返す
        onCreated={(test) =>
          setList((prev) => [test, ...prev].sort((a, b) => b.date.localeCompare(a.date)))
        }
      />
    </div>
  );
}
