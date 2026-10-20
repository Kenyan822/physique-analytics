# 全置換（PUT）を1文の SQL で書く

## トランザクションを持てない層で「消して入れる」をどう原子的にするか

**状況**: その日の種目リスト（`session_exercises`）を、並び順ごと全置換する PUT を書いた。
素直に書くと `delete` → `insert` の2文になる。ところが `repository` の `DBTX` には
`Begin` が無く、2文の間で失敗するとリストが空になる。

**判断**: データ変更 CTE（`with ... as (insert ... on conflict do update)`）に `delete` を
つなげ、**1文**にした。

```go
const qw = `
	with upsert as (
		insert into session_exercises (session_id, exercise_id, item_order)
		select $1, t.id, t.ord
		from unnest($2::uuid[]) with ordinality as t(id, ord)
		on conflict (session_id, exercise_id) do update set item_order = excluded.item_order
	)
	delete from session_exercises
	where session_id = $1 and exercise_id <> all($2::uuid[])`
```

**理由**: Postgres は1文なら原子的に実行する。`unnest ... with ordinality` が配列の添字を
そのまま `item_order` にしてくれるので、Go 側でループして `insert` を N 回投げずに済む
（N+1 にならない）。トランザクションを引き回す設計に変える前に、1文で足りないかを先に見る。

## `[]uuid.UUID` を `any($1)` に渡さない。空でも nil にしない

```go
list := make([]string, 0, len(ids)) // nil にしない
for _, id := range ids {
	list = append(list, id.String())
}
rows, err := r.db.Query(ctx, q, sessionID, list) // ... exercise_id <> all($2::uuid[])
```

- 本番は `QueryExecModeExec`（Supavisor の transaction mode でプリペアドが使えない）。
  pgx が要素の型をサーバに問い合わせられないので、**`[]string` を渡して SQL 側で
  `::uuid[]` にキャスト**する。
- **nil のスライスは NULL になる。** `x <> all(NULL)` は NULL（偽扱い）なので、
  「全部外す」のつもりが**何も消えない／何も弾けない**。`make([]string, 0, n)` で空配列にする。
  Python の `[]` と `None` の違いに近いが、Go は nil スライスが普通に `len == 0` で動くため
  気づきにくい。

## 空配列と「キーが無い」を区別する

**状況**: `{"exerciseIds": []}`（全部外す）と `{}`（送り忘れ）を分けたかった。

**判断**: 生成された型は `ExerciseIds []uuid.UUID`。JSON に配列があれば空でも **非 nil**、
キーが無ければ **nil** にデコードされる。`req.Body.ExerciseIds == nil` で送り忘れだけを 422 にした。

**理由**: `len(x) == 0` で判定すると両者が同じになる。Go の `encoding/json` は
`[]` を `[]T{}`（非 nil・長さ0）にデコードするので、nil との区別が付く。
逆に**レスポンス側**は nil を `null` で出してしまうので、`[]openapi.SessionExercise{}` で
初期化して `[]` を返す（クライアントが「行が0件」を `null` と取り違えない）。

## 外部キー違反を 500 にしない

存在しない種目 ID をそのまま `insert` すると FK 違反で 500 になる。「送った内容が仕様に合わない」は
422 なので、**書く前に `count(*)` で数えて**、合わなければ `ErrInvalid` を返す。
repository は HTTP を知らないので、`errors.Is(err, repository.ErrInvalid)` をハンドラが 422 に写す
（`ErrNotFound` → 404 と同じ作り）。
