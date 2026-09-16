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

| | | 要件 |
|---|---|---|
| `/` | 今日の記録。種目別のセットとトン数 | — |
| `/log` | **トレーニングの入力** | T-01〜T-07 |
| `/meals` | 食事の入力と残量 | N-01〜N-05 / N-07 |
| `/body` | 体重・体脂肪率・周囲長 | B-02 / B-03 / B-06 |
| `/plan` | 計画ブロックと月次目標 | P-02 / P-03 |
| `/photos` | 身体写真のタイムライン（閲覧のみ） | B-07 |
| `/blood` | 血液検査の記録と基準外の表示 | B-08 |
| `/settings` | 身長・起点・フェーズ・栄養設定・テンプレート・大会 | P-01 / P-04〜P-06 |

`/log` が本体。`/photos` は保存先（R2）が未設定のあいだ、設定方法を出して
中身は空になる（[ADR-0008](../docs/adr/0008-r2-photo-storage.md)）。

| 要件 | 内容 |
|---|---|
| **T-02** | 種目を選ぶと**前回の重量・レップ・RIR が初期値に入る**。前回の推定1RM も出す |
| T-03 | 数値は +/- ボタンで刻む（重量 2.5kg / レップ 1）。**数字をタップすれば直接打てる**。RIR は 0〜4 を1タップ |
| T-04 | 記録すると次のセット番号になり、入力欄は開いたまま |
| T-05 | インターバルタイマー。多関節は 180 秒、単関節は 90 秒（種目ごとに上書き可） |
| T-06 | メニュー（テンプレート）を選ぶと、その日の種目だけに絞られる |
| **T-07** | **オフラインで記録できる。** 端末に積んで、復帰時に自動で送る。
Service Worker により**オフラインの状態で新規に開ける** |

**+/- ボタンを主役にしているのは**、片手・手袋・汗の状態でキーボードを出すのが
現実的でないため。ただし 60 → 82.5 のように飛んだ値はボタンでは遠すぎるので、
数字をタップして直接打つ経路も残してある。

入力中の値は**文字列のまま持つ**。数値に直しながら保持すると「82.」と打った時点で
82 に丸められ、続きの小数が打てない。読み取りは確定時にまとめてやる。

**画面を閉じて開き直しても続きから入力できる。** その日の記録を先に読み、
セット番号は「その種目の最大 + 1」で決める。

## 構成

| | |
|---|---|
| `app/` | App Router。Server Component でデータを取り、Client Component で入力する |
| `app/<画面>/actions.ts` | Server Actions。**API を叩くのはここだけ** |
| `lib/api/client.ts` | API クライアント。生成された型を使う薄い層 |
| `lib/api/server.ts` | サーバ側専用のクライアント。`server-only` を付けている |
| `lib/api/schema.gen.ts` | openapi.yaml からの生成物。編集しない |
| `lib/jst.ts` | JST の日付（[ADR-0013](../docs/adr/0013-timezone-jst.md)） |
| `lib/offline/` | オフラインの記録キュー（要件 T-07） |
| `public/sw.js` | Service Worker。オフラインでもページを開けるようにする |

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
| `NEXT_PUBLIC_SUPABASE_URL` | Supabase のプロジェクト URL。**ブラウザに載る**（公開前提） |
| `NEXT_PUBLIC_SUPABASE_ANON_KEY` | Supabase の anon key。**ブラウザに載る**（公開前提）。単体では何も読めない |

## Next.js 16

`AGENTS.md` に「このバージョンは破壊的変更がある。書く前に
`node_modules/next/dist/docs/` を読め」と書かれている。これは `next dev` が
自動で書き戻すファイルなので、消さずにコミットしておく。
