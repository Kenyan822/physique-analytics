# 時刻と「今日」

このアプリの時刻はすべて JST（[ADR-0013](../adr/0013-jst.md)）。
`timeutil` に寄せてある。

## `Truncate(24 * time.Hour)` で「今日」を作らない

```go
// 壊れている
timeutil.Now().Truncate(24 * time.Hour)
```

`timeutil.Now()` は JST を返すのに、**`Truncate` はロケーションを見ない。**
エポックからの絶対時刻を切るので、JST では **9時** に丸まる。

```
2026-10-14 06:53 JST = 2026-10-13 21:53 UTC
          → Truncate(24h) → 2026-10-13 00:00 UTC = 2026-10-13 09:00 JST
```

**朝9時前に呼ぶと前日になる。** トレ前の朝食、起床直後の体重、朝の写真。
一番使う時間帯で外れる。

```go
// JST で年月日を取り出して組み直す
func StartOfDay(t time.Time) time.Time {
	j := t.In(JST)

	return time.Date(j.Year(), j.Month(), j.Day(), 0, 0, 0, 0, time.UTC)
}
```

### 返す時刻のロケーションは UTC

`JST` のまま返すと**逆方向に1日ずれる。**

`date` 列は日付そのもので、時刻もタイムゾーンも持たない。
`2028-06-15 00:00 +09:00` を渡すと UTC に直されて `2028-06-14 15:00` になり、
`DATE` にキャストされて **6月14日**になる。

`time.Parse("2006-01-02", s)` が返すのと同じ **UTC の深夜**に揃える。

実際にこれで既存のテストが落ちた。`ParseInLocation(..., JST)` に替えたら、
入力した日付の行が見つからなくなった。

### 5箇所で踏んだ

[#311](https://github.com/Kenyan822/physique-analytics/issues/311)。
最初に気づいたのは MCP の `log_meal` で、issue には2箇所と書いた。
**grep したら5箇所あった。**

```
handler/photo.go        写真のガイド
handler/plan_block.go   実測からの引き直し
mcpserver/weekly.go     weekly_actions の既定日
mcpserver/correlation.go 相関の期間
mcpserver/mcpserver.go  query の既定の期間
```

`mcpserver.go` には **「『今日』は JST で決める（ADR-0013）。UTC だと日付が
1日ずれる」というコメントが付いていた。** 分かっていて踏んでいる。
`timeutil.Now()` が JST を返すので、そこで安心してしまう。

## 型でも lint でも防げないので、ソースを走査する

この間違いは

- **型では防げない。** `time.Time` のまま、正しく見える
- **golangci-lint の forbidigo でも防げない。** 関数名しか見ず、引数を見ない。
  `Truncate(24 * time.Hour)` の `24 * time.Hour` が対象外になる

残る手段がソースの走査しかなかった。

```go
var truncateDay = regexp.MustCompile(`Truncate\(24\s*\*\s*time\.Hour\)`)

func TestTruncateは使わない(t *testing.T) {
	// api/ 以下の .go を読んで、コメント以外の行に出てきたら落とす
}
```

**ガードを書いたら、実際に落ちることを確かめる。** 1箇所わざと戻して走らせた。

```
--- FAIL: TestTruncateは使わない
    「今日」は timeutil.Today() / StartOfDay() で出す。
      ../../internal/mcpserver/correlation.go:58
```

通らないガードは、無いより悪い（守られていると思い込む）。

### ソースを走査するテストの作法

- **生成物（`gen/`）は除く。** 直す先が違う
- **コメント行は許す。** 「使ってはいけない」と書けなくなる
- **自分自身と、理由を書いた実装（`today.go`）は除く**
