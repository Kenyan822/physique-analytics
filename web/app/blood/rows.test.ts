import { describe, expect, it } from "vitest";

import { emptyRow, toItems, type ItemRow } from "./rows";

function row(over: Partial<ItemRow> = {}): ItemRow {
  return { ...emptyRow(), name: "ヘモグロビン", value: "15.2", unit: "g/dL", ...over };
}

describe("toItems", () => {
  it("数値として読める値は value に入れる", () => {
    const [item] = toItems([row()]);

    expect(item.value).toBe(15.2);
    expect(item.textValue).toBeNull();
  });

  it("数値で読めない値は textValue に入れる", () => {
    // 検査票には「陰性」「(−)」のような項目がある
    const [item] = toItems([row({ value: "陰性" })]);

    expect(item.value).toBeNull();
    expect(item.textValue).toBe("陰性");
  });

  it("全角の数字は数値として読む", () => {
    expect(toItems([row({ value: "１５．２" })])[0].value).toBe(15.2);
  });

  it("項目名が空の行は送らない", () => {
    // 入力途中の空行で 422 にしない
    expect(toItems([row(), emptyRow(), row({ name: "  " })])).toHaveLength(1);
  });

  it("基準範囲は入っているものだけ送る", () => {
    const [item] = toItems([row({ refLow: "13.5", refHigh: "" })]);

    expect(item.refLow).toBe(13.5);
    expect(item.refHigh).toBeNull();
  });

  it("単位が空なら null", () => {
    expect(toItems([row({ unit: " " })])[0].unit).toBeNull();
  });

  it("値が空でも項目名があれば送る（採血したが結果が無い項目）", () => {
    const [item] = toItems([row({ value: "" })]);

    expect(item.value).toBeNull();
    expect(item.textValue).toBeNull();
  });

  it("検査票の並び順を保つ", () => {
    const got = toItems([row({ name: "A" }), emptyRow(), row({ name: "B" })]);

    expect(got.map((i) => i.name)).toEqual(["A", "B"]);
  });

  it("前後の空白を落とす", () => {
    expect(toItems([row({ name: " 尿酸 " })])[0].name).toBe("尿酸");
  });
});
