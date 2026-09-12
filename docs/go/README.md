# Go の学び

このプロジェクトは Go の学習を兼ねている（[ADR-0006](../adr/0006-go-backend.md)）。実装中に学んだことをトピック別に記録する。

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
| [project-layout.md](project-layout.md) | `go.mod` の `go` と `toolchain`、`internal/`、`go tool` によるツール管理 |
| [directory-layout.md](directory-layout.md) | **api/ の構成**。各パッケージの役割、依存の向き、ファイルの分け方 |
| [http-server.md](http-server.md) | `ServeMux`、タイムアウト、グレースフルシャットダウン、`log/slog` |
| [testing.md](testing.md) | `_test` パッケージ、`t.Context()`、インターフェースを使ったスタブ |
| [tdd.md](tdd.md) | **TDD の実際の流れ**。Red の見方、層ごとのテスト方針、詰まったときの切り分け |
| [database.md](database.md) | pgx、トランザクションでのテスト隔離、`DBTX` インターフェース |
| [mcp.md](mcp.md) | MCP サーバ。ツールの説明文の書き方、読み取り専用 SQL の守り方 |

## 想定しているトピック

実装中に該当する場面が来たら作る。**先回りして書かない。**

- `error-handling.md` — `errors.Is` / `errors.As`、wrapping、panic を使わない理由
- `interfaces.md` — 小さいインターフェース、受け手側で定義する慣習
- `context.md` — キャンセル伝播、タイムアウト、値の受け渡し
- `concurrency.md` — goroutine とチャネル、`sync` パッケージ、レースの検出
- `numeric.md` — 浮動小数点の扱い、単位を型で表現する方法（分析ロジックで必要になる）
