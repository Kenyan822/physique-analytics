# 期間の判定を「引く・突き合わせる」に分ける

日別の達成フラグ（`GET /v1/streaks`）で学んだこと。

## 1日ごとにクエリを投げず、期間を数クエリで引いて Go で突き合わせる

```go
// 食事の合計・筋トレをした日・目標の履歴。3クエリで366日ぶんが揃う
in, err := s.streaks.Inputs(ctx, from, to)
days := analytics.BuildStreak(from, to, in.Targets, in.Consumed, in.Trained)
```

日ごとに「その日の目標」「その日の食事」を引くと 366日で千往復を超える。
**SQL は集計（`group by date`）と存在確認だけ**にして、判定は純粋関数に寄せる。
判定が DB に依存しないので、境界値（±10% ちょうど）をテーブル駆動で網羅できる。

## 「その日に有効な1行」は昇順の履歴を1回なぞって引く

履歴は `starts_on` の昇順で来る。日付を昇順に進めながら、ポインタを進めるだけで済む。

```go
cur := -1
for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
	date := d.Format(time.DateOnly)
	for cur+1 < len(targets) && targets[cur+1].StartsOn <= date {
		cur++ // YYYY-MM-DD は文字列比較で日付の順になる
	}
	// cur == -1 なら、その日に効いている目標は無い
}
```

日ごとに二分探索しなくてよい。`from` の日に効いている行は `from` より前に始まっているので、
SQL では `starts_on <= to` で取り、`from` では絞らない。

## 「未達」と「判定できない」を `*bool` で分ける

`bool` だと目標が引けない日が `false`（未達）に見える。`*bool` の `nil` を JSON の `null` にして区別する。
Python の `Optional[bool]` と同じだが、Go ではポインタで表すので、**呼び出し側が必ず nil を見る**ことになる。

## 浮動小数の境界は余裕を持たせる

```go
math.Abs(actual-target) <= target*0.10 + 1e-9
```

`180 * 1.1` は `198.00000000000003`。ちょうど +10% の 198 が「外れ」にならないよう、
許容に `1e-9` の余裕を足す。`==` で比べないのと同じ理由。

## `numeric` の合計は `::float8`、リテラルの 0 は `0::numeric`

```sql
coalesce(sum(protein_g), 0::numeric)::float8
```

本番は `QueryExecModeExec`（プリペアドを使わない）なので、pgx が列の型をサーバに問い合わせない。
`numeric` を `float64` に読むには SQL 側で `::float8` に揃える。`coalesce` の第2引数に
裸の `0` を書くと型が合わずに落ちることがあるので `0::numeric`。

## 1本のコネクションで2つの `Rows` を同時に開けない

```go
meals, _ := r.db.Query(ctx, q1)
defer meals.Close()
for meals.Next() { ... }
meals.Close()          // ← 次のクエリの前に閉じる
trained, _ := r.db.Query(ctx, q2)
```

トランザクション（`pgx.Tx`）は1本の接続で、前の `Rows` を閉じるまで次の `Query` が
`conn busy` になる。`defer` は関数末尾まで閉じないので、**ループを読み終えた時点で明示的に `Close`** する
（`Close` は何度呼んでもよい）。
