# 任意のクエリで SQL の条件を切り替える

## `before` を足しても省略時の挙動を変えない

**状況**: `GET /v1/exercises/{id}/last-performance` に、その日を含まず前で最新を返す
`before`（任意の日付）を足した。省略時は今までどおり最新を返す必要がある。

**判断**: クエリ文字列を `if` で組み立てず、SQL 側で `null` を許す1本にした。

```go
// openapi.yaml が before を `*openapi_types.Date` にする。省略は nil
where s.exercise_id = $1 and ws.deleted_at is null
  and ($2::date is null or ws.date < $2::date)

rows, err := r.db.Query(ctx, q, exerciseID, dateOrNil(before))
```

**理由**: 条件ごとに SQL を文字列連結すると、分岐の数だけ別のクエリになりテストが増える。
`$n::date is null or ...` なら1本で、**プレースホルダの型を `::date` で固定**できる。
本番は `QueryExecModeExec`（プリペアドを使わない）なので、nil を渡したときに pgx が
型を推論できず、キャストが無いと落ちる。Python の `Optional[date]` を `None` で渡すのと
同じ形だが、Go は**ポインタの nil が NULL になる**点が違う（`time.Time` のゼロ値は
`0001-01-01` として渡ってしまい、条件が「全部真」にならない）。

## 「その日を含まない」は `<` で、日付の型は date

`ws.date < $2::date`。`<=` にすると今日が混ざる。`date` 列は JST の日付そのもの
（ADR-0013）なので、`timestamptz` にキャストして時刻で比べない。

## 不正な日付は 422 ではなく 400 になる

oapi-codegen の生成コードがクエリを `openapi_types.Date` にパースし、失敗すると
`RequestErrorHandlerFunc` に落ちる。このリポジトリではそこを 400（problem+json）に
している。**ハンドラに検証を書かない**。生成コードが弾く範囲はスキーマで決まる
（`format: date`）ので、仕様側に書けば実装は増えない。
