# 適用開始日つきの履歴を引く

摂取目標を履歴にしたとき（[ADR-0018](../adr/0018-versioned-nutrition-targets.md)）に学んだこと。

## 「その日に有効な1行」は `<= order by desc limit 1`

```go
const q = `
	select id, name, starts_on, protein_g, fat_g, carb_g, created_at
	from manual_targets
	where starts_on <= $1::date
	order by starts_on desc
	limit 1`
```

「有効かどうか」のフラグを持たず、**開始日の並びから導く**。0件は `pgx.ErrNoRows` で、
このコードでは「目標なし」であってエラーではない。

```go
e, err := scan(r.db.QueryRow(ctx, q, d))
if errors.Is(err, pgx.ErrNoRows) {
	return nil, nil // 呼び出し側が自動計算にフォールバックする
}
```

## `date` 列に `time.Time` を渡さず、暦日の文字列にする

**状況**: JST の「今日」を `date` 列と比べたい。`timeutil.Now()` は JST の `time.Time`。

**判断**: `t.Format(time.DateOnly)` の文字列を渡し、SQL 側で `$1::date` と書く。

**理由**: このプロジェクトは本番と同じ `QueryExecModeExec` で繋ぐ。`time.Time` は
タイムスタンプ文字列として送られ、サーバ側の接続タイムゾーンで解釈されるので、
**JST 0:00〜9:00 に UTC の前日として扱われる余地がある**。暦日を文字列で渡せば、
どのタイムゾーンでも同じ日になる。`::date` は型の推論を固定する意味もある。

Python の `date` 型のように「日付だけの型」が標準に無い（`time.Time` は常に時刻を持つ）のが
根にある。

## `on conflict (列) do update` は一意制約がある列に対して書く

```go
const q = `
	insert into manual_targets (starts_on, protein_g, fat_g, carb_g)
	values ($1::date, $2, $3, $4)
	on conflict (starts_on) do update set
		protein_g = excluded.protein_g, fat_g = excluded.fat_g, carb_g = excluded.carb_g
	returning id, name, starts_on, protein_g, fat_g, carb_g, created_at`
```

「同じ日に2回変えたら上書き」を `unique` 制約で保証し、アプリ側の
「あるか調べてから入れる」を書かない（競合する）。`excluded` は入れようとした行。
**更新しない列（`name`）は `set` に書かない**。書かなければ既存の値が残る。

## 空のスライスは `nil` ではなく `[]T{}` で作る

```go
out := []openapi.ManualTargetEntry{}   // var out []... だと JSON が null になる
```

`nil` スライスは JSON で `null`。クライアントが `items.map` で落ちる。
ハンドラ側でも、スタブが `nil` を返したときに備えて `[]` に直した（テストが見つけた）。

## `rows.Close()` は `defer`、`rows.Err()` はループの後

```go
rows, err := r.db.Query(ctx, q)
if err != nil { ... }
defer rows.Close()
for rows.Next() { ... }
if err := rows.Err(); err != nil { ... }   // Next が false で終わった理由
```

`Next()` が false を返すのは「終わり」と「エラー」の両方。区別するのが `rows.Err()`。
