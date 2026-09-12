# api

physique-analytics の API サーバ（Go）。

型は [`../openapi.yaml`](../openapi.yaml) が唯一の正で、このディレクトリの `gen/` はそこからの生成物
（[ADR-0007](../docs/adr/0007-openapi-schema-driven.md)）。**手で型を書かない。**

## 構成

| | |
|---|---|
| `cmd/server/` | API サーバ。設定読み込みと配線だけ |
| `cmd/mcp/` | MCP サーバ（stdio） |
| `internal/config/` | 環境変数 → 設定 |
| `internal/database/` | Postgres 接続（pgxpool） |
| `internal/handler/` | `openapi.StrictServerInterface` の実装 |
| `internal/timeutil/` | JST 固定（[ADR-0013](../docs/adr/0013-timezone.md)） |
| `internal/analytics/` | 分析ロジック（[ADR-0011](../docs/adr/0011-go-analytics.md)）。純粋関数だけ |
| `internal/csvio/` | CSV の読み書き。`data/sample/*.csv` とスキーマを一致させる |
| `internal/auth/` | Supabase の JWT 検証（ES256 / JWKS） |
| `internal/mcpserver/` | MCP のツール（[ADR-0010](../docs/adr/0010-mcp-over-analysis-ui.md)） |
| `gen/openapi/` | openapi.yaml からの生成物。編集しない |
| `migrations/` | golang-migrate の SQL |

## 動かす

```bash
docker compose up -d                      # リポジトリルートで Postgres を起動
docker compose run --rm migrate up        # マイグレーション適用

cd api
DATABASE_URL='postgres://physique:dev@localhost:5432/physique?sslmode=disable' \
  AUTH_DISABLED=true go run ./cmd/server
curl -s localhost:8080/health
```

本番と同じイメージで確認するなら `docker compose --profile full up api`。

## 環境変数

| 変数 | 既定 | 内容 |
|---|---|---|
| `DATABASE_URL` | （必須） | Postgres の接続文字列。本番は Supavisor（port 6543） |
| `PORT` | `8080` | Cloud Run が注入する |
| `DATABASE_MAX_CONNS` | `5` | 1インスタンスが張る接続数の上限 |
| `SUPABASE_JWKS_URL` | （必須） | JWT 検証用の公開鍵。`https://<ref>.supabase.co/auth/v1/.well-known/jwks.json` |
| `AUTH_DISABLED` | `false` | `true` で認証を切る。**ローカル開発専用** |

## コード生成

```bash
go generate ./...
```

`openapi.yaml` を変えたら必ず実行してコミットする。CI が最新性を検証する。

## 実装状況

`/health` / `exercises` / `workout-sessions` / `workout-sets` / `last-performance` /
`templates` / `sync`（pull / push）/ `export/csv` / `import/csv`。
**openapi.yaml の24操作すべてを実装済み。** 仕様に操作を足すと `Server` が
`StrictServerInterface` を満たさなくなりビルドが落ちるので、追随漏れはコンパイル時に分かる。
`openapi.yaml` に操作を足すと `Server` がインターフェースを満たさなくなりビルドが落ちるので、
仕様と実装のずれはコンパイル時に分かる。

## MCP サーバ

Claude Code から分析するためのサーバ（[ADR-0010](../docs/adr/0010-mcp-over-analysis-ui.md)）。
手元で動かし、DB を直接読む。

```bash
cd api && go build -o /tmp/physique-mcp ./cmd/mcp
claude mcp add physique --env DATABASE_URL='postgres://physique:dev@localhost:5432/physique?sslmode=disable' -- /tmp/physique-mcp
```

| ツール | 内容 |
|---|---|
| `list_exercises` | 種目マスタ |
| `list_workout_sessions` | 期間指定でトレーニング記録 |
| `weekly_volume` | 部位別の週間セット数 + MEV/MRV 判定 |
| `exercise_progress` | 種目の推定1RM の推移と傾き |
| `query` | **読み取り専用の SQL**。事前に定義できない分析用 |

詳細は [docs/go/mcp.md](../docs/go/mcp.md)。
