# web

入力と最小限のサマリ表示（[ADR-0009](../docs/adr/0009-web-first-with-input.md)）。
**分析UIは作らない** —— 事前に定義できない質問には画面で答えられないため、
分析は MCP 経由で行う（[ADR-0010](../docs/adr/0010-mcp-over-analysis-ui.md)）。

型は [`../openapi.yaml`](../openapi.yaml) が唯一の正で、`lib/api/schema.gen.ts` は
そこからの生成物（[ADR-0007](../docs/adr/0007-openapi-schema-driven.md)）。**手で型を書かない。**

## 動かす

API が先に要る。

```bash
# リポジトリルートで
docker compose up -d
docker compose run --rm migrate up
cd api && DATABASE_URL='postgres://physique:dev@localhost:5432/physique?sslmode=disable' \
  AUTH_DISABLED=true go run ./cmd/server
```

```bash
cd web
cp .env.example .env.local
pnpm install
pnpm dev
```

**npm / yarn は使わない**（`preinstall` で弾かれる）。理由は [CLAUDE.md](../CLAUDE.md)。

## 画面

| | |
|---|---|
| `/` | 今日の記録。種目別のセットとトン数 |
| `/log` | **入力**（要件 T-01〜T-04） |

`/log` が本体。

| 要件 | 内容 |
|---|---|
| **T-02** | 種目を選ぶと**前回の重量・レップ・RIR が初期値に入る**。前回の推定1RM も出す |
| T-03 | 数値は +/- ボタンで刻む（重量 2.5kg / レップ 1）。RIR は 0〜4 を1タップ |
| T-04 | 記録すると次のセット番号になり、入力欄は開いたまま |
| T-05 | インターバルタイマー。多関節は 180 秒、単関節は 90 秒（種目ごとに上書き可） |
| T-06 | メニュー（テンプレート）を選ぶと、その日の種目だけに絞られる |
| **T-07** | **オフラインで記録できる。** 端末に積んで、オンライン復帰時に自動で送る |

数値入力にキーボードを使わないのは、片手・手袋・汗の状態で現実的でないため。

**画面を閉じて開き直しても続きから入力できる。** その日の記録を先に読み、
セット番号は「その種目の最大 + 1」で決める。

## 構成

| | |
|---|---|
| `app/` | App Router。Server Component でデータを取り、Client Component で入力する |
| `app/log/actions.ts` | Server Actions。API を叩くのはここだけ |
| `lib/api/client.ts` | API クライアント。生成された型を使う薄い層 |
| `lib/api/server.ts` | サーバ側専用のクライアント。`server-only` を付けている |
| `lib/api/schema.gen.ts` | openapi.yaml からの生成物。編集しない |
| `lib/jst.ts` | JST の日付（[ADR-0013](../docs/adr/0013-timezone-jst.md)） |
| `lib/offline/` | オフラインの記録キュー（要件 T-07） |

## コマンド

```bash
pnpm dev         # 開発サーバ
pnpm gen         # openapi.yaml → lib/api/schema.gen.ts
pnpm test        # vitest
pnpm typecheck   # tsc --noEmit
pnpm lint
pnpm build
```

`openapi.yaml` を変えたら `pnpm gen` を実行してコミットする。CI が最新性を検証する。

## 環境変数

| 変数 | 内容 |
|---|---|
| `API_BASE_URL` | API の場所。**サーバ側でだけ使う**（ブラウザに露出しない） |
| `API_TOKEN` | Supabase の JWT。API 側の認証を有効にしている場合に要る |

## Next.js 16

`AGENTS.md` に「このバージョンは破壊的変更がある。書く前に
`node_modules/next/dist/docs/` を読め」と書かれている。これは `next dev` が
自動で書き戻すファイルなので、消さずにコミットしておく。
