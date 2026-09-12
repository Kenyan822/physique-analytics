# api

physique-analytics の API サーバ（Go）。

型は [`../openapi.yaml`](../openapi.yaml) が唯一の正で、このディレクトリの `gen/` はそこからの生成物
（[ADR-0007](../docs/adr/0007-openapi-schema-driven.md)）。**手で型を書かない。**

## 構成

| | |
|---|---|
| `cmd/server/` | エントリポイント。設定読み込みと配線だけ |
| `internal/config/` | 環境変数 → 設定 |
| `internal/database/` | Postgres 接続（pgxpool） |
| `internal/handler/` | `openapi.StrictServerInterface` の実装 |
| `internal/timeutil/` | JST 固定（[ADR-0013](../docs/adr/0013-timezone.md)） |
| `gen/openapi/` | openapi.yaml からの生成物。編集しない |
| `migrations/` | golang-migrate の SQL |

## 動かす

```bash
docker compose up -d                      # リポジトリルートで Postgres を起動
docker compose run --rm migrate up        # マイグレーション適用

cd api
DATABASE_URL='postgres://physique:dev@localhost:5432/physique?sslmode=disable' go run ./cmd/server
curl -s localhost:8080/health
```

本番と同じイメージで確認するなら `docker compose --profile full up api`。

## 環境変数

| 変数 | 既定 | 内容 |
|---|---|---|
| `DATABASE_URL` | （必須） | Postgres の接続文字列。本番は Supavisor（port 6543） |
| `PORT` | `8080` | Cloud Run が注入する |
| `DATABASE_MAX_CONNS` | `5` | 1インスタンスが張る接続数の上限 |

## コード生成

```bash
go generate ./...
```

`openapi.yaml` を変えたら必ず実行してコミットする。CI が最新性を検証する。

## 実装状況

`/health` のみ実装済み。他の操作は `internal/handler/unimplemented.go` が 501 を返す。
`openapi.yaml` に操作を足すと `Server` がインターフェースを満たさなくなりビルドが落ちるので、
仕様と実装のずれはコンパイル時に分かる。
