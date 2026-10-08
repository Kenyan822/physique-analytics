# 制約違反を 500 にしない

本番で `meals` の CHECK 制約が新しい値（`rough`）を知らず、クライアントが送ると 500
「サーバ内部エラー」になった（#263）。原因は送った側にあるのに、画面からもログからも分からなかった。

## `pgconn.PgError` は `errors.As` で取り出し、コードで判別する

```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23514" { ... }
```

**メッセージ文字列で判別しない。** 文言は言語設定や Postgres のバージョンで変わる。
SQLSTATE（`23514` check / `23503` foreign key / `23505` unique）は仕様で固定されている。
`errors.As` は `%w` で包まれた連鎖も辿るので、`fmt.Errorf("…: %w", err)` を挟んでも当たる。

Python（psycopg）の `except CheckViolation` のように型で分かれていないので、**コードの比較が要る**。
`pgx` は例外ではなく戻り値のエラーに `*PgError` を入れて返す。

## 各所で変換せず、応答を作る1か所で受ける

既存は「一意制約違反 → `ErrConflict` → 409」を各リポジトリが個別に変換していた。
制約違反は全テーブルで起こりうるので、全部に書くと漏れる（今回がそう）。
**`ResponseErrorHandlerFunc`（ハンドラがエラーを返したときの受け皿）で判別する**と、
新しいテーブルを足しても自動で効く。個別の変換（`ErrConflict`・`ErrNotFound`）はそのまま優先される —
ハンドラが先に 404/409 を返すので、受け皿には届かない。

## detail には制約名だけを出す

```text
値が許されていない（meals_source_check）
```

Postgres のメッセージ（`new row for relation "meals" violates …` と **失敗した行の値**）をそのまま
返さない。制約名だけで「どの値が問題か」は分かる。テーブルの列・SQL・送られた値は応答に含めない
（テストで `relation` / `SQLSTATE` / 送った値が含まれないことを固定している）。

## テストは実際に違反を起こす

```go
_, err := testdb.Begin(t).Exec(ctx, `insert into meals (date, source) values ('2032-05-01', 'guess')`)
```

`PgError{Code: "23514"}` を手で組み立てると、**本当にそのコードが返るか**を確かめていない。
実 DB で起こせば、制約名（`meals_source_check` など）も実物で固定できる。
違反するとトランザクションが中断される（`current transaction is aborted`）ので、**1テスト1違反**にする。
