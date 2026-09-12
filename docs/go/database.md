# データベース（pgx）

## `*pgxpool.Pool` を直接受けない

```go
// これだと テストで本物の DB しか使えない
func NewExercise(pool *pgxpool.Pool) *Exercise

// インターフェースにすると pgx.Tx も渡せる
func NewExercise(db DBTX) *Exercise
```

```go
// *pgxpool.Pool と pgx.Tx の両方が満たす
type DBTX interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}
```

**状況**: 3つのテストを `t.Parallel()` で走らせたら、件数のアサーションが +1 されて落ちた。
片方のテストが挿入した行を、もう片方が数えていた。

**判断**: `DBTX` を挟み、テストはトランザクション内で走らせて最後に Rollback する。

**理由**: 未コミットの変更は他のトランザクションから見えない。これで並行しても干渉しない。
`t.Cleanup` で `delete` を書く必要もなくなる。sqlc も同じ形のインターフェースを生成するので、
後で sqlc に移ってもこの構造のまま。

## `pgx.ErrNoRows` を上の層に漏らさない

```go
e, err := scanExercise(r.db.QueryRow(ctx, q, id))
if errors.Is(err, pgx.ErrNoRows) {
	return openapi.Exercise{}, fmt.Errorf("種目 %s: %w", id, ErrNotFound)
}
```

ハンドラが `pgx` を import しなくて済む。DB を差し替えたときの影響が repository で止まる。

`%w` で包むと `errors.Is(err, ErrNotFound)` が通る。`%v` にすると切れるので注意。

## `NULL` の可能性がある条件を SQL 側で表現する

```go
const q = `
	select ... from exercises
	where ($1::muscle_group is null or muscle_group = $1::muscle_group)
	  and ($2::boolean or deleted_at is null)`
```

Go 側で文字列を組み立てると、条件が増えるたびに分岐が増える。
`$1 is null or ...` の形にすれば、フィルタが無いときも同じクエリで済む。

**キャストが要る**理由: プレースホルダだけでは Postgres が型を決められない。
`$1::muscle_group` と書かないと `could not determine data type of parameter` になる。

## `Scan` の引数順は列の順と一致させる

```go
const exerciseColumns = `id, name, muscle_group, is_compound, default_rest_sec,
	created_at, updated_at, deleted_at`

err := r.Scan(&e.Id, &e.Name, &mg, &e.IsCompound, &e.DefaultRestSec,
	&e.CreatedAt, &e.UpdatedAt, &e.DeletedAt)
```

**列リストを定数にして使い回す。** `select *` にすると、マイグレーションで列が増えた瞬間に
`Scan` がずれる。定数にしておけば変更箇所が1つで済む。

型が合わないと**実行時**に落ちる（コンパイルでは分からない）。ここは repository の
テストで本物の DB に繋いで確かめる領域。

## `pgx.Row` と `pgx.Rows` を1つの関数で扱う

```go
// Scan さえできればよい
type row interface {
	Scan(dest ...any) error
}

func scanExercise(r row) (openapi.Exercise, error) { ... }
```

`QueryRow` は `pgx.Row`、`Query` のループは `pgx.Rows` を返すが、
必要なのは `Scan` だけ。最小のインターフェースを自分で定義すれば
1件取得と一覧で同じ関数を使える。

## timestamptz は UTC で返る

```go
e.CreatedAt = e.CreatedAt.In(timeutil.JST)
```

`timestamptz` は内部的に UTC で保持され、pgx も UTC の `time.Time` を返す。
アプリは JST 固定（ADR-0013）なので、scan した直後に変換する。

コンテナの `TZ` 環境変数に頼らない。distroless には tzdata が入っておらず、
`time.LoadLocation` が失敗する。`time.FixedZone` を使う。

## Supavisor（transaction mode）ではプリペアドステートメントを使わない

```go
pc.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
```

pgx は既定（`QueryExecModeCacheStatement`）でプリペアドステートメントを
自動キャッシュするが、transaction mode のプーラーでは接続をまたげず
`prepared statement "stmt1" does not exist` で落ちる。

**低負荷では物理接続が1本しかないので再現しない。** アクセスが増えて
接続が散った瞬間に壊れる。詳細は `private/learning/postgres-connections.md`。
