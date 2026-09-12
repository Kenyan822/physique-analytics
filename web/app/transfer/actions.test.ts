import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const revalidatePath = vi.fn();
const importCsvApi = vi.fn();
const exportCsvApi = vi.fn();

vi.mock("next/cache", () => ({ revalidatePath }));
vi.mock("@/lib/api/server", () => ({
  serverApi: () => ({ importCsv: importCsvApi, exportCsv: exportCsvApi }),
}));

const { exportCsv, importCsv } = await import("./actions");
const { ApiError } = await import("@/lib/api/client");

describe("importCsv", () => {
  beforeEach(() => {
    revalidatePath.mockClear();
    importCsvApi.mockReset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("FormData をそのまま渡す（読み直さない）", async () => {
    importCsvApi.mockResolvedValue({ imported: 2, skipped: 0, errors: [] });
    const form = new FormData();
    form.set("resource", "daily");

    const res = await importCsv(form);

    expect(importCsvApi).toHaveBeenCalledWith(form);
    expect(res).toEqual({ ok: true, result: { imported: 2, skipped: 0, errors: [] } });
  });

  it("取り込んだら全画面を再検証する", async () => {
    importCsvApi.mockResolvedValue({ imported: 1, skipped: 0, errors: [] });

    await importCsv(new FormData());

    // 日次も食事もトレーニングも変わりうる
    expect(revalidatePath).toHaveBeenCalledWith("/", "layout");
  });

  it("422 はフィールド名つきで返す", async () => {
    importCsvApi.mockRejectedValue(
      new ApiError(422, {
        type: "about:blank",
        status: 422,
        title: "入力が不正",
        errors: [{ field: "file", message: "ヘッダが違う" }],
      }),
    );

    const res = await importCsv(new FormData());

    expect(res).toEqual({ ok: false, message: "file: ヘッダが違う" });
  });

  it("失敗したら再検証しない", async () => {
    importCsvApi.mockRejectedValue(new ApiError(500));

    await importCsv(new FormData());

    expect(revalidatePath).not.toHaveBeenCalled();
  });
});

describe("exportCsv", () => {
  beforeEach(() => exportCsvApi.mockReset());

  it("空の期間は送らない", async () => {
    exportCsvApi.mockResolvedValue("date\n");

    await exportCsv("daily", "", "");

    // 空文字を送ると「空で絞る」と解釈されうる
    expect(exportCsvApi).toHaveBeenCalledWith({
      resource: "daily",
      from: undefined,
      to: undefined,
    });
  });

  it("指定した期間は送る", async () => {
    exportCsvApi.mockResolvedValue("date\n");

    await exportCsv("workouts", "2029-01-01", "2029-01-31");

    expect(exportCsvApi).toHaveBeenCalledWith({
      resource: "workouts",
      from: "2029-01-01",
      to: "2029-01-31",
    });
  });

  it("本文をそのまま返す", async () => {
    exportCsvApi.mockResolvedValue("date,weight_kg\n2029-01-05,74.2\n");

    const res = await exportCsv("daily", "", "");

    expect(res).toEqual({ ok: true, csv: "date,weight_kg\n2029-01-05,74.2\n" });
  });
});
