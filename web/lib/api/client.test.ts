import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";

import { ApiError, createClient } from "./client";

const BASE = "http://api.test";

function jsonResponse(body: unknown, status = 200, contentType = "application/json") {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": contentType },
  });
}

describe("createClient", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("GET のパスとクエリを組み立てる", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ items: [] }));
    const api = createClient({ baseUrl: BASE });

    await api.listExercises({ muscleGroup: "胸", includeDeleted: false });

    const [url] = fetchMock.mock.calls[0];
    const parsed = new URL(url as string);
    expect(parsed.pathname).toBe("/v1/exercises");
    // 日本語はエンコードされる。生で載せると環境によって壊れる
    expect(parsed.searchParams.get("muscleGroup")).toBe("胸");
    expect(parsed.searchParams.get("includeDeleted")).toBe("false");
  });

  it("未指定のクエリは送らない", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ items: [] }));
    const api = createClient({ baseUrl: BASE });

    await api.listExercises({});

    const [url] = fetchMock.mock.calls[0];
    expect(new URL(url as string).search).toBe("");
  });

  it("トークンがあれば Authorization を付ける", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ items: [] }));
    const api = createClient({ baseUrl: BASE, token: "tok" });

    await api.listExercises({});

    const [, init] = fetchMock.mock.calls[0];
    expect((init as RequestInit).headers).toMatchObject({
      Authorization: "Bearer tok",
    });
  });

  it("トークンが無ければ Authorization を付けない", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ items: [] }));
    const api = createClient({ baseUrl: BASE });

    await api.listExercises({});

    const [, init] = fetchMock.mock.calls[0];
    expect((init as RequestInit).headers).not.toHaveProperty("Authorization");
  });

  // RFC 7807。API はエラーをこの形で統一している
  it("problem+json を ApiError にして投げる", async () => {
    // Response のボディは一度しか読めない。呼ぶたびに作り直す
    fetchMock.mockImplementation(() =>
      Promise.resolve(
        jsonResponse(
          { type: "about:blank", title: "種目が見つからない", status: 404 },
          404,
          "application/problem+json",
        ),
      ),
    );
    const api = createClient({ baseUrl: BASE });

    await expect(api.getExercise("00000000-0000-0000-0000-000000000000")).rejects.toThrow(ApiError);

    try {
      await api.getExercise("00000000-0000-0000-0000-000000000000");
    } catch (e) {
      const err = e as ApiError;
      expect(err.status).toBe(404);
      expect(err.problem?.title).toBe("種目が見つからない");
      // メッセージだけ見ても何が起きたか分かる
      expect(err.message).toContain("種目が見つからない");
    }
  });

  // problem+json で返ってこないエラー（LB の 502 など）でも落ちてはいけない
  it("problem+json でないエラーも ApiError にする", async () => {
    fetchMock.mockResolvedValue(new Response("<html>502</html>", { status: 502 }));
    const api = createClient({ baseUrl: BASE });

    await expect(api.listExercises({})).rejects.toMatchObject({ status: 502 });
  });

  it("204 はボディを読まない", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    const api = createClient({ baseUrl: BASE });

    await expect(api.deleteExercise("11111111-1111-1111-1111-111111111111")).resolves.toBeUndefined();
  });

  it("POST は JSON として送る", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ id: "x", name: "自作", muscleGroup: "胸" }, 201));
    const api = createClient({ baseUrl: BASE });

    await api.createExercise({ name: "自作", muscleGroup: "胸" });

    const [url, init] = fetchMock.mock.calls[0];
    expect(new URL(url as string).pathname).toBe("/v1/exercises");
    const req = init as RequestInit;
    expect(req.method).toBe("POST");
    expect(req.headers).toMatchObject({ "Content-Type": "application/json" });
    expect(JSON.parse(req.body as string)).toEqual({ name: "自作", muscleGroup: "胸" });
  });

  it("パスパラメータをエスケープする", async () => {
    fetchMock.mockResolvedValue(jsonResponse({}));
    const api = createClient({ baseUrl: BASE });

    await api.getExercise("a/b");

    const [url] = fetchMock.mock.calls[0];
    expect(new URL(url as string).pathname).toBe("/v1/exercises/a%2Fb");
  });

  it("baseUrl の末尾スラッシュを吸収する", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ items: [] }));
    const api = createClient({ baseUrl: `${BASE}/` });

    await api.listExercises({});

    const [url] = fetchMock.mock.calls[0];
    expect(new URL(url as string).pathname).toBe("/v1/exercises");
  });
});
