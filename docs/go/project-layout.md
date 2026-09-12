# プロジェクト構成とモジュール

## `go.mod` の `go` と `toolchain` は別物

```
module github.com/Kenyan822/physique-analytics/api

go 1.27          // このモジュールが要求する最低バージョン（言語機能もこれで決まる）
toolchain go1.27.1  // ビルドに使うコンパイラのバージョン
```

**状況**: 手元の Go は 1.24.5 だが、[docs/06-技術選定.md](../06-技術選定.md) では 1.27 を採用と決めていた。
Go のサポート対象は最新2世代だけなので、1.24 はすでにサポート外。

**判断**: `go 1.27` / `toolchain go1.27.1` を書いた。手元の Go を入れ替えていない。

**理由**: Go 1.21 以降、`go.mod` の `go` が手元の Go より新しいとき、
コマンドは必要な toolchain を自動でダウンロードして実行する。
`go version` は 1.24.5 のままでも、`go build` は 1.27.1 で走る。

```console
$ go version
go version go1.24.5 darwin/arm64

$ go build ./...     # 裏で go1.27.1 を取得して使う
```

無効化したいときは `GOTOOLCHAIN=local`。CI では `setup-go` の `go-version-file: api/go.mod` を
指定して、バージョンを go.mod だけで管理している（二重管理を避けるため）。

## `internal/` は import を制限する

`internal/` の下にあるパッケージは、`internal/` の親ディレクトリより外から import できない。
コンパイラが強制するので、規約ではなく仕様。

```
api/
  cmd/server/       # main。ここは薄く保つ
  internal/
    config/         # 環境変数 → 設定
    database/       # Postgres 接続
    handler/        # openapi の実装
    timeutil/       # JST 固定
  gen/openapi/      # openapi.yaml からの生成物
  migrations/       # SQL
```

`api/internal/handler` は `api/` 配下からしか import できない。
公開したい API を後から絞るのは難しいので、**まず internal に置いて、外から要るとわかったら出す**。

## `cmd/` は複数のバイナリを置くための慣習

`cmd/server/main.go` → `go build ./cmd/server` で `server` ができる。
ディレクトリ名がバイナリ名になる。MCP サーバを足すときは `cmd/mcp/` を作る。

## ツールの依存は `go.mod` に書ける（Go 1.24〜）

```console
$ go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
```

go.mod の末尾に `tool` 行が入り、`go tool oapi-codegen` で実行できる。

```
tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
```

**利点**: バージョンが go.mod に固定される。以前は `tools.go` に `_ "..."` と import を並べる
回避策が必要だった。開発者ごとにバージョンが違う、という事故が起きない。

**落とし穴**: ツールの依存がモジュール全体の依存グラフに入る。
golang-migrate の CLI を `go get -tool` したところ、対応する全 DB ドライバ
（MySQL / ClickHouse / Spanner / Neo4j …）を引き込んで依存が数百に膨らんだので取り下げ、
公式の Docker イメージを compose から呼ぶ形にした。

**判断基準**: 依存の軽いツール（oapi-codegen）は `go tool`、重いツールはコンテナ。
