import type { BloodTestItem } from "@/lib/api/client";

/** 入力中の1行。**すべて文字列で持つ**。数値に直すのは送る直前 */
export type ItemRow = {
  name: string;
  value: string;
  unit: string;
  refLow: string;
  refHigh: string;
};

export function emptyRow(): ItemRow {
  return { name: "", value: "", unit: "", refLow: "", refHigh: "" };
}

/**
 * 入力欄を API に送る形にする（要件 B-08）。
 *
 * **項目名が空の行は落とす。** 入力途中の空行でサーバに 422 を返させない。
 * **並び順は保つ。** 検査票の順に読めないと突き合わせができない。
 */
export function toItems(rows: ItemRow[]): BloodTestItem[] {
  return rows
    .filter((r) => r.name.trim() !== "")
    .map((r) => {
      const value = toNumber(r.value);
      // 数値で読めない値（「陰性」など）は文字のまま残す
      const text = r.value.trim();

      return {
        name: r.name.trim(),
        value,
        textValue: value === null && text !== "" ? text : null,
        unit: r.unit.trim() === "" ? null : r.unit.trim(),
        refLow: toNumber(r.refLow),
        refHigh: toNumber(r.refHigh),
      };
    });
}

/** 空欄・数値で読めないものは null。日本語入力のままだと全角数字になる */
function toNumber(text: string): number | null {
  const normalized = text.normalize("NFKC").trim();
  if (normalized === "") return null;

  const n = Number(normalized);

  return Number.isFinite(n) ? n : null;
}
