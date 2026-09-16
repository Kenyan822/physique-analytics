# oapi-codegen が何を作り、どう繋がるか

`openapi.yaml` が型の唯一の正（[ADR-0007](../adr/0007-openapi-schema-driven.md)）。
**Go の型を手で書かない。** その仕組み。

生成物が 3,279 行あり、`api/` のコードの半分以上を占める。

## 生成を起動する

```go
// api/gen/gen.go
package gen

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../openapi.yaml
```

```console
$ cd api && go generate ./...
```

`go:generate` は**コメントであってコードではない**。`go build` では走らない。
`go generate` が全ファイルを走査して、この行を見つけて実行する。

### `go tool` で呼んでいる

Go 1.24 から、ツールの依存を `go.mod` に書ける。

```
tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
```

`go run github.com/...@latest` と違い、**バージョンが `go.sum` で固定される**。
手元と CI で同じものが動く（[project-layout.md](project-layout.md)）。

### `gen` パッケージは空

`gen.go` には `package gen` と `//go:generate` しかない。
**生成コマンドを1か所に集める**ためだけのファイル。
散らばっていると「どこかの生成を忘れた」が起きる。

## 設定

```yaml
package: openapi
output: openapi/openapi.gen.go
generate:
  models: true
  std-http-server: true
  strict-server: true
```

### `std-http-server` —— 標準の `net/http` に出す

`chi-server` / `echo-server` / `gin-server` も選べる。標準を選んだのは
Go 1.22 の `ServeMux` がメソッドとパス変数を扱えるようになり、
**ルータのために依存を足す理由が消えた**から。

### `strict-server` —— ここが本質

これが無いと、ハンドラの署名はこうなる。

```go
// strict-server なし
ListExercises(w http.ResponseWriter, r *http.Request, params ListExercisesParams)
```

`w` を持っているので、**仕様に無いステータスや形を書ける**。
`openapi.yaml` に 200 と 404 しか書いていなくても 418 を返せてしまう。

有効にすると、

```go
// strict-server あり
ListExercises(ctx context.Context, request ListExercisesRequestObject) (ListExercisesResponseObject, error)
```

`ListExercisesResponseObject` は**このエンドポイントで許された応答だけ**を
満たすインターフェース。仕様にない形を返そうとするとコンパイルが通らない。

### `embedded-spec` は切ってある

```yaml
# embedded-spec は有効にしない。
# 生成物に spec を埋め込むと kin-openapi が本番バイナリにリンクされる。
# 実行時のリクエスト検証を入れるまで不要で、入れるときに改めて検討する。
```

埋め込むと `openapi.yaml` を base64 で持ったバイナリになり、
`kin-openapi` がリンクされる。実行時検証をしないなら重いだけ。

**副作用として、`security:` が実行時に効かない。** `/health` の素通しは
`auth.Middleware` にパスを渡して実現している
（[request-flow.md](request-flow.md#health-を素通しする理由)）。

## 生成される4種類

### 1. モデル

```go
type Exercise struct {
    CreatedAt       time.Time         `json:"createdAt"`
    DefaultRestSec  *int              `json:"defaultRestSec,omitempty"`
    Id              uuid.UUID         `json:"id"`
    MuscleGroup     MuscleGroup       `json:"muscleGroup"`
    ...
}
```

**必須は値、任意はポインタ。** `openapi.yaml` の `required` がそのまま出る。
`*int` が nil なら「送られていない」で、0 とは区別される。

#### enum は `x-enum-varnames` が要る

```yaml
MuscleGroup:
  type: string
  enum: [胸, 背中, 肩, ...]
  x-enum-varnames: [Chest, Back, Shoulders, ...]
```

書かないと定数名が `N1` `N2` … になって読めない。
値は日本語のまま、**識別子だけ英語**にする。

### 2. `ServerInterface` と `ServerInterfaceWrapper`

素の `http.HandlerFunc` 相当と、クエリ／パス変数を構造体に詰める層。

```go
err = runtime.BindQueryParameterWithOptions("form", true, false, "muscleGroup",
    r.URL.Query(), &params.MuscleGroup, ...)
```

### 3. `StrictServerInterface`

```go
type StrictServerInterface interface {
    GetHealth(ctx context.Context, request GetHealthRequestObject) (GetHealthResponseObject, error)
    ListExercises(ctx context.Context, request ListExercisesRequestObject) (ListExercisesResponseObject, error)
    ...
}
```

`handler.Server` がこれを実装する。

### 4. レスポンス型と `Visit` メソッド

```go
type ListExercises200JSONResponse struct {
    Items []Exercise `json:"items"`
}

func (response ListExercises200JSONResponse) VisitListExercisesResponse(w http.ResponseWriter) error {
    var buf bytes.Buffer
    if err := json.NewEncoder(&buf).Encode(response); err != nil { return err }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(200)
    _, err := buf.WriteTo(w)
    return err
}
```

**ステータスコードが型に埋まっている。** `200JSONResponse` を返せば 200。
`w.WriteHeader` を自分で書く場所が無い。

## 実装漏れをコンパイルで落とす

```go
var _ openapi.StrictServerInterface = (*Server)(nil)
```

`openapi.yaml` に操作を足して `go generate` すると、
**`StrictServerInterface` にメソッドが1つ増える**。実装を忘れると、
この行でビルドが落ちる。

`(*Server)(nil)` は「nil の `*Server`」。**実体を作らずに型だけ確認する**
イディオムで、実行時のコストはゼロ（変数は最適化で消える）。

## CI が最新性を検証する

生成物はコミットする。CI で再生成して差分が出たら落とす。

```
openapi.yaml を編集 → go generate 忘れ → 型が古いまま → CI が検出
```

**コミットしない選択肢もある**が、そうすると

- エディタの補完が効かない（生成前は型が無い）
- `go build` の前に必ず `go generate` が要る
- コードレビューで API の変更が見えない

ので、コミットする方を選んでいる。

## 手書きとの境界

```
openapi.yaml
    │ go generate
    ▼
gen/openapi/openapi.gen.go        ← 編集しない
    │ 実装する
    ▼
internal/handler/*.go             ← 手書き
    │ 呼ぶ
    ▼
internal/repository/*.go          ← 手書き。SQL
```

**リポジトリも `openapi` の型を返している。**

```go
func (r *Exercise) List(ctx context.Context, f ExerciseFilter) ([]openapi.Exercise, error)
```

DB 用の型を別に作って詰め替える設計もあるが、**列と API のフィールドが
ほぼ1対1**なので、変換の層を作る利益が無い。

代償として、`repository` が `gen/openapi` に依存する。
API のスキーマを変えると SQL の `Scan` も直すことになるが、
**どのみち直す必要がある**ので実害は出ていない。

## 他の言語との対応

| | 生成するもの | 実行 |
|---|---|---|
| Go | モデル + サーバ | `cd api && go generate ./...` |
| TypeScript | 型のみ（`schema.gen.ts`） | `cd web && pnpm gen` |
| Swift | **生成していない**（手書き） | — |

Swift だけ手書きなのは、`APIClient` が 281 行で生成器を入れるコストに
見合っていないため（[docs/swift/app-structure.md](../swift/app-structure.md#生成コードが無い)）。
ずれたら `swift test` の enum 一致テストが落ちる。
