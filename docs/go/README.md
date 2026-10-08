# Go の学び

このプロジェクトは Go の学習を兼ねている（[ADR-0006](../adr/0006-go-backend.md)）。実装中に学んだことをトピック別に記録する。

iOS 側は [docs/swift/](../swift/)。

## 何を書くか

**学習目的なので幅広く書く。** 自分の言葉で書くこと自体が学習になり、検索し直すより手元にある方が速い。

- 文法・標準ライブラリの使い方（`defer` の評価タイミング、スライスの挙動など）
- イディオム（なぜ Go ではこう書くのか）
- Python / TypeScript と判断が異なった点
- ハマった点と、その原因

## エントリの形式

内容に応じて2種類を使い分ける。

### A. 文法・イディオムのメモ

コード例と短い説明。

````markdown
## スライスの append は元の配列を書き換えることがある

```go
a := []int{1, 2, 3}
b := append(a[:1], 4)   // a も [1 4 3] に変わる
```

容量に余裕があると同じ配列を再利用するため。切り出して渡すときは
`slices.Clone` するか、`a[:1:1]` で容量を切る。
````

### B. 判断の記録

設計や実装方針で迷った場合。

```markdown
## <何をしたか / 何にハマったか>

**状況**: どのコードを書いていて、何が起きたか
**判断**: どう書いたか
**理由**: なぜそう書くのか。他言語との違いがあれば併記
```

## 索引

| トピック | 内容 |
|---|---|
| [request-flow.md](request-flow.md) | **リクエストが入ってから返るまで**。配線・認証・生成物・ハンドラ・SQL を1本の線で |
| [codegen.md](codegen.md) | **oapi-codegen** が何を作り、どう繋がるか。strict server の効き目 |
| [project-layout.md](project-layout.md) | `go.mod` の `go` と `toolchain`、`internal/`、`go tool` によるツール管理 |
| [directory-layout.md](directory-layout.md) | **api/ の構成**。各パッケージの役割、依存の向き、ファイルの分け方 |
| [http-server.md](http-server.md) | `ServeMux`、タイムアウト、グレースフルシャットダウン、`log/slog` |
| [testing.md](testing.md) | `_test` パッケージ、`t.Context()`、インターフェースを使ったスタブ、`t.Parallel()` とトランザクションの相性 |
| [tdd.md](tdd.md) | **TDD の実際の流れ**。Red の見方、層ごとのテスト方針、詰まったときの切り分け |
| [database.md](database.md) | pgx、トランザクションでのテスト隔離、`DBTX` インターフェース、部分インデックスへの upsert |
| [mcp.md](mcp.md) | MCP サーバ。ツールの説明文の書き方、読み取り専用 SQL の守り方 |
| [interfaces.md](interfaces.md) | nil ポインタと interface、使う側での定義、共通化の判断 |
| [numeric.md](numeric.md) | 定数と引数の切り分け、`*float64` で「未測定」を表す、従属変数のクランプ |
| [routine-cycle.md](routine-cycle.md) | **巡回するルーティン**。状態を持たずに「今日は何日目か」を導く。`%` の符号 |
| [replace-all.md](replace-all.md) | **全置換の PUT**。`Begin` の無い層で1文の CTE にする、`[]string` + `::uuid[]`、nil スライスと空配列、FK 違反を 422 にする |
| [versioned-lookup.md](versioned-lookup.md) | **適用開始日つきの履歴を引く**。`date` 列に `time.Time` を渡す落とし穴、`on conflict (列)` の upsert、`Rows.Close` |
| [streak-calendar.md](streak-calendar.md) | **期間の判定を「引く・突き合わせる」に分ける**。`*bool` で「未達」と「判定できない」を分ける、浮動小数の境界、`Rows` を同時に開けない |
| [optional-query.md](optional-query.md) | **任意のクエリ**。`$n::date is null or ...` で SQL を1本に保つ、nil ポインタが NULL になる、不正な値は生成コードが 400 にする |
| [rounding-invariant.md](rounding-invariant.md) | **丸めても壊れない不変条件を、丸め方で守る**。誤差を1か所に集める、有限入力は全数テスト、`math.Round` と Python の `round` の違い |
| [constraint-violation.md](constraint-violation.md) | **制約違反を 500 にしない**。`errors.As` で `PgError` を SQLSTATE で判別、応答を作る1か所で受ける、detail は制約名だけ、実際に違反を起こすテスト |

## 想定しているトピック

実装中に該当する場面が来たら作る。**先回りして書かない。**

- `error-handling.md` — `errors.Is` / `errors.As`、wrapping、panic を使わない理由
- `context.md` — キャンセル伝播、タイムアウト、値の受け渡し
- `concurrency.md` — goroutine とチャネル、`sync` パッケージ、レースの検出
