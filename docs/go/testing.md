# テスト

## テストパッケージは `_test` 接尾辞を付ける

```go
package handler_test   // package handler ではない
```

`handler_test` パッケージからは、`handler` の**公開されているものしか触れない**。
外から使える形になっているかがテストを書いた時点で分かる。
内部関数を直接テストしたくなったら、たいてい切り出すべきパッケージがある、というサイン。

## `t.Context()`（Go 1.24〜）

```go
req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil)
```

テストの終了時に自動で cancel される context が取れる。
`context.Background()` を渡すとリークしたゴルーチンが止まらないことがある。

`noctx` linter は `httptest.NewRequest`（context なし）を検出して
`NewRequestWithContext` を使えと言ってくる。

## `t.Parallel()`

```go
func TestGetHealth_DBが応答すれば200を返す(t *testing.T) {
	t.Parallel()
	...
}
```

同じ関数内の `t.Parallel()` を呼んだテストが並行に走る。
グローバル変数や共有リソースを触るテストには付けない。

## テスト名に日本語を使える

```go
func TestGetHealth_DBが応答しなければ503とProblemを返す(t *testing.T) {
```

Go の識別子は Unicode の文字を許すので通る。`go test -run` でも指定できる。
「何を保証しているか」がテスト名だけで読めるので、このプロジェクトでは日本語で書く。

## インターフェースはテスト側の都合で切る

```go
// handler 側
type Pinger interface {
	Ping(ctx context.Context) error
}
```

`*pgxpool.Pool` をそのまま受け取ると、ヘルスチェックのテストに実 DB が要る。
**使う側（handler）が必要な操作だけのインターフェースを定義する**のが Go の慣習。
実装側（database パッケージ）はインターフェースを知らないまま、たまたま満たしている。

```go
type stubPinger struct{ err error }

func (p stubPinger) Ping(context.Context) error { return p.err }
```

Java/C# のように `implements` と書かないので、**テスト用のスタブが4行で済む**。
モックライブラリを入れる場面がかなり減る。

## コンパイル時にインターフェース充足を確認する

```go
var _ openapi.StrictServerInterface = (*Server)(nil)
```

`Server` が `StrictServerInterface` を満たさなくなった瞬間にビルドが落ちる。
openapi.yaml に操作を足して実装を忘れた、を検出できる。
実行時のコストはゼロ（変数は最適化で消える）。

## `-race` を付ける

```console
$ go test ./... -race -cover
```

データ競合を実行時に検出する。CI では必ず付ける。
遅くなる（数倍）が、競合はローカルでは再現せず本番でだけ壊れる類のバグなので割に合う。
