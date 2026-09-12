/**
 * オフラインでもアプリを開けるようにする（要件 T-07）。
 *
 * **ジムに着いてから開くのが普通。** 記録キュー（lib/offline/）があっても、
 * ページ自体が開けなければ意味がない。
 *
 * 方針:
 *   - 画面の HTML … ネットワーク優先、失敗したらキャッシュ（stale-while-revalidate 的）
 *   - Next.js の静的資産 … キャッシュ優先（ハッシュ付きなので古くならない）
 *   - API … キャッシュしない。記録は毎回最新が要る
 */
const CACHE = "physique-v1";

// オフラインで最初に開く可能性があるページ
const PAGES = ["/", "/log"];

self.addEventListener("install", (event) => {
  // 新しい sw をすぐ有効にする。古いキャッシュを抱えたまま待たない
  self.skipWaiting();

  event.waitUntil(
    caches.open(CACHE).then((cache) =>
      // 1つ失敗しても install 全体を落とさない。
      // 認証が要る状態だと /log が 401 になることがある
      Promise.allSettled(PAGES.map((p) => cache.add(p))),
    ),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (event) => {
  const { request } = event;

  // GET 以外は触らない。記録の POST をキャッシュすると二重送信になる
  if (request.method !== "GET") return;

  const url = new URL(request.url);

  // 別オリジン（API など）は触らない
  if (url.origin !== self.location.origin) return;

  // Server Actions / RSC のペイロードはキャッシュしない。
  // 古い状態を返すと画面と実データがずれる
  if (url.searchParams.has("_rsc")) return;

  // ハッシュ付きの静的資産。内容が変われば URL も変わるのでキャッシュ優先でよい
  if (url.pathname.startsWith("/_next/static/")) {
    event.respondWith(cacheFirst(request));
    return;
  }

  // 画面の HTML
  if (request.mode === "navigate") {
    event.respondWith(networkFirst(request));
  }
});

async function cacheFirst(request) {
  const cached = await caches.match(request);
  if (cached) return cached;

  const response = await fetch(request);
  if (response.ok) {
    const cache = await caches.open(CACHE);
    cache.put(request, response.clone());
  }

  return response;
}

async function networkFirst(request) {
  try {
    const response = await fetch(request);
    if (response.ok) {
      const cache = await caches.open(CACHE);
      cache.put(request, response.clone());
    }

    return response;
  } catch {
    // オフライン。最後に成功した内容を返す。
    // 種目マスタは滅多に変わらないので、古くても入力には十分
    const cached = await caches.match(request);
    if (cached) return cached;

    // そのページのキャッシュも無い場合は /log を出す。入力だけはさせる
    const fallback = await caches.match("/log");
    if (fallback) return fallback;

    return new Response("オフラインです。一度オンラインで開くと次回から使えます。", {
      status: 503,
      headers: { "Content-Type": "text/plain; charset=utf-8" },
    });
  }
}
