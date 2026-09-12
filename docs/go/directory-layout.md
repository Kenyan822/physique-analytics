# ディレクトリ構成

`api/` の中身と、**なぜその分け方なのか**。
Go の基本（`internal/` の意味、`cmd/` の慣習）は [project-layout.md](project-layout.md)。

## 全体

```
api/
├── cmd/
│   └── server/          main。設定読み込みと配線だけ
├── gen/
│   ├── gen.go           go:generate のエントリポイント
│   ├── oapi-codegen.yaml
│   └── openapi/         openapi.yaml からの生成物。編集しない
├── internal/
│   ├── analytics/       分析ロジック（e1RM / FFMI / TDEE）。純粋関数だけ
│   ├── config/          環境変数 → 設定
│   ├── database/        Postgres 接続（pgxpool）
│   ├── handler/         openapi.StrictServerInterface の実装
│   ├── repository/      SQL
│   ├── testdb/          DB を使うテストの共通処理
│   └── timeutil/        JST 固定
├── migrations/          golang-migrate の SQL
├── Dockerfile
├── .golangci.yaml
└── go.mod
```

規模（2026-09-13 時点）:

| | 実装 | テスト |
|---|---:|---:|
| `internal/handler` | 749 | 949 |
| `internal/repository` | 733 | 723 |
| `internal/analytics` | 116 | 205 |
| `internal/config` | 56 | 61 |
| `internal/database` | 65 | — |
| `internal/testdb` | 115 | — |
| `internal/timeutil` | 14 | — |
| `cmd/server` | 77 | — |
| `gen/openapi`（生成物） | 3279 | — |

**生成物がコードの半分以上を占める。** `openapi.yaml` を正にした結果で、
型とルーティングを手で書いていない証拠でもある。

## 依存の向き

```
cmd/server
    │
    ├──▶ config      （環境変数を読む）
    ├──▶ database    （接続プールを作る）
    │
    └──▶ handler ──▶ repository ──▶ database
              │           │
              │           └──▶ timeutil
              ├──▶ analytics
              └──▶ gen/openapi
```

**下の層は上を知らない。** `repository` は `handler` を import しないし、
`analytics` は何も import しない（`math` だけ）。

循環参照はコンパイルエラーになるので、Go では規約ではなく仕様として守られる。

## 各パッケージの役割

### `cmd/server` — 配線だけ

設定を読み、接続プールを作り、ハンドラに渡して起動する。それだけ。

```go
srv := &http.Server{
	Addr:    net.JoinHostPort("", strconv.Itoa(cfg.Port)),
	Handler: handler.NewRouter(handler.New(pool, repository.NewExercise(pool.DB()), repository.NewWorkout(pool.DB()))),
	ReadHeaderTimeout: 10 * time.Second,
}
```

**ここにロジックを書かない。** `main` パッケージはテストから import できないので、
書いた分だけテストできない領域が増える。

### `internal/analytics` — 純粋関数だけ

```go
func E1RM(weight, reps, rir float64) float64
func NormFFMI(lbm, heightCm float64) float64
func EstimateTDEE(avgIntakeKcal, slopeKgPerWeek float64) float64
func LinearSlope(xs, ys []float64) (slope float64, ok bool)
```

DB も HTTP も知らない。**引数と戻り値だけ。** import しているのは `math` のみ。

分析ロジックの正はここ（[ADR-0011](../adr/0011-go-analytics.md)）。
`reference/analysis/analyze.py` は移植の検証基準として残しており、
**テストケースは Python 側のテストから写している**。期待値を独自に立てると
「一致すること」を確かめられなくなる。

仕様が [docs/03-分析ロジック.md](../03-分析ロジック.md) に明文化されているので、
**TDD が最も機能する領域**。テスト209行に対して実装116行。

### `internal/repository` — SQL

テーブルごとにファイルを分ける。

```
repository.go          共通（ErrNotFound / ErrConflict / DBTX）
exercise.go            種目の読み取り
exercise_write.go 相当は exercise.go にまとめている
workout.go             セッション・セット・前回値
```

`*pgxpool.Pool` ではなく `DBTX` インターフェースを受ける。
テストをトランザクション内で走らせるため（[database.md](database.md)）。

`pgx.ErrNoRows` を上に漏らさず、`ErrNotFound` に変換する。
ハンドラが `pgx` を import しなくて済む。

### `internal/handler` — 変換だけ

やることは3つ。

1. リクエストを `repository` の入力型に詰め替える
2. `openapi.yaml` の制約のうち生成コードが見ないもの（数値の範囲）を検証する
3. `repository` のエラーを HTTP ステータス + Problem に変換する

```go
switch {
case repository.IsConflict(err):
	return ...409..., nil
case repository.IsNotFound(err):
	return ...404..., nil
case err != nil:
	return nil, err
}
```

**ビジネスロジックを書かない。** 計算は `analytics`、データの整合性は
`repository` と DB の制約。ここに条件分岐が増えてきたら、どちらかに
置くべきものが混ざっている。

### `internal/testdb` — テストの足場

`Begin(t)` がトランザクションを開き、テスト終了時に Rollback する。
DB に繋がらなければ `t.Skip`（手元で `go test ./...` が赤くならないように）。

**`internal/` に置いているので、リポジトリの外から import できない。**
テスト用のヘルパが公開 API に漏れない。

### `internal/timeutil` — 14行

```go
var JST = time.FixedZone("JST", 9*60*60)
func Now() time.Time { return time.Now().In(JST) }
```

これだけのためにパッケージを切っている。[ADR-0013](../adr/0013-timezone-jst.md) で
「時刻はすべて JST 固定」と決めており、**`time.Now()` を直接呼ぶ場所を無くす**のが目的。
`grep -r "time.Now()" internal/` で違反を見つけられる状態にしておきたい。

`time.LoadLocation("Asia/Tokyo")` を使わないのは、distroless イメージに
tzdata が入っておらず実行時に失敗するため。

### `gen/openapi` — 触らない

`openapi.yaml` からの生成物。直したくなったら**直す先は仕様か生成設定**。

CI が「再生成して差分が出ないか」を検証しているので、手で編集しても
次の CI で落ちる。

## ファイルの分け方

### 1テーブル = 1ファイル、読み書きは分けない

```
repository/exercise.go        List / Get / Create / Update / SoftDelete
repository/exercise_test.go   読み取りのテスト
repository/exercise_write_test.go  書き込みのテスト
```

**実装は分けず、テストは分けている。** 実装を CQRS 的に分けても、
同じテーブルの列定義を2箇所で持つことになって壊れやすい。
テストは PR 単位で増えるので、分かれていた方が差分が読みやすい。

### ハンドラは openapi の tag に合わせる

```
handler/health.go            tags: [health]
handler/exercise.go          tags: [exercises]
handler/workout.go           tags: [workouts]
handler/last_performance.go  tags: [workouts] だが独立させた
```

`last_performance.go` だけ tag から外しているのは、**要件 T-02 の本体**で
`analytics` を使う唯一のハンドラだから。`workout.go` に混ぜると埋もれる。

### `unimplemented.go`

`openapi.StrictServerInterface` の全メソッドを 501 で返すスタブ。
`Server` がこれを埋め込むことで、実装済みの操作だけを上書きすれば足りる。

**`openapi.yaml` に操作が増えると `Server` がインターフェースを満たさなくなり
ビルドが落ちる。** 「仕様に足したのにサーバ側が追随していない」を
コンパイル時に検出できる。

## まだ無いもの

| | いつ作るか |
|---|---|
| `cmd/mcp/` | MCP サーバ（[ADR-0010](../adr/0010-mcp-over-analysis-ui.md)） |
| `internal/auth/` | Supabase の JWT 検証 |
| `internal/csv/` | CSV の入出力（要件 I-01） |

**先回りしてディレクトリだけ作らない。** 空のパッケージは
「何かあるはず」と読み手を惑わせる。
