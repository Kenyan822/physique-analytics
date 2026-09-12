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

## 部分インデックスに対する upsert は `on conflict ... where` が要る

```sql
create unique index daily_metrics_date_unique on daily_metrics (date) where deleted_at is null;
```

```go
// 論理削除した行を除いた一意制約なので、where を書かないと
// 「一致する制約が無い」で落ちる
const q = `
	insert into daily_metrics (date, weight_kg) values ($1, $2)
	on conflict (date) where deleted_at is null do update set ...`
```

`on conflict (date)` だけでは、Postgres は「`date` 単独の一意制約」を探して
`there is no unique or exclusion constraint matching the ON CONFLICT specification`
を返す。**部分インデックスを狙うときは、インデックス定義と同じ `where` を書く。**

論理削除（ADR-0014）と upsert を併用すると必ずこの形になる。

## 「送らなかった項目は変えない」は SQL 側の `coalesce` で表す

```go
type DailyInput struct {
	Date     openapi_types.Date
	WeightKg *float32   // nil は「変更しない」
	Kcal     *int
}
```

```sql
on conflict (date) where deleted_at is null do update set
	weight_kg = coalesce(excluded.weight_kg, daily_metrics.weight_kg),
	kcal      = coalesce(excluded.kcal, daily_metrics.kcal)
```

`do update set weight_kg = excluded.weight_kg` にすると、送らなかった項目が
null で潰れる。朝に体重だけ入れて夜に食事を入れると、体重が消える。

**Go 側で「absent」と「null」を区別しようとしない。** `omitempty` の付いた
`*T` はどちらも nil になり、JSON に key があったかは復元できない
（区別するには生の `json.RawMessage` を持つか `**T` にする必要がある）。
そこまでするより、**null を「変更しない」と定義して仕様に書く**方が単純で、
消したい場合は DELETE を用意すれば足りる。

## `openapi_types.Date` は `time.Time` の埋め込み

```go
d.Date.Format("2006-01-02")   // ○ 埋め込みのメソッドがそのまま使える
d.Date.Time.Format(...)       // × staticcheck QF1008 が出る

row.Scan(&d.Date.Time)        // ○ Scan は埋め込みフィールドを直接渡す
```

メソッド呼び出しは埋め込みのプロモーションで解決されるので `.Time` は要らない。
一方 `Scan` はポインタで書き込むため、`&d.Date` では `sql.Scanner` の実装が
無く失敗する。**読むときは省略、書くときは `.Time` を明示**と覚える。

## `min` / `max` は組み込み関数なので引数名に使わない

```go
func checkInt(v *int, lo, hi int) string   // ○
func checkInt(v *int, min, max int) string // × revive: redefines-builtin-id
```

Go 1.21 で `min` / `max` が組み込みになった。シャドウしてもコンパイルは
通るが、同じ関数の中で組み込みの `min` が呼べなくなる。lint が止める。
