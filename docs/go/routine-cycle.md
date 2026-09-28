# 巡回するルーティン

「今日は何日目か」を決める（要件 T-01 / #232）。`internal/routine`。

## 状態を持たずに導く

**カウンタを持たない。** 直近のセッションから計算する。

```go
func Today(days []Day, last *Session, today string) *Day {
    if last != nil && last.Date == today {
        if d := find(days, last.TemplateID); d != nil { return d }
    }
    i := indexOf(days, last.TemplateID)
    if i < 0 { return &days[0] }

    return &days[(i+1)%len(days)]
}
```

`next_day_order` のような列を持つと、**記録を消したときにずれる。**
実データから出せば、整合しない状態がそもそも作れない。

同じ考え方は `mealslot`（時刻から区分を導く）でも採った。
**保存できる値でも、導けるなら導く。**

## 曜日で決めない

| | |
|---|---|
| 曜日固定 | 休むとその部位が飛ぶ。週によって偏る |
| **巡回** | **やった日だけ進む。日程が乱れてもバランスが崩れない** |

トレーニングの設計上、部位ごとの週あたりセット数にレンジがある。
曜日固定だと休んだ週に下限を割る。

## 今日の途中では切り替えない

```go
if last != nil && last.Date == today {
    // その日のまま
}
```

**入力中に並びが変わると混乱する。** 日付が変わるまで動かさない。

## `%` は負になりうるが、ここでは安全

```go
return &days[(i+1)%len(days)]
```

Go の `%` は**被除数の符号に従う**（`-1 % 3 == -1`）。
Python の `%` は常に非負なので、移植すると添字が負になって落ちる。

ここは `i >= 0` を先に確かめてあるので `i+1 >= 1`。安全だが、
**符号が入りうる場所では `((i%n)+n)%n` にする。**

## 連番を前提にしない

```go
type Day struct {
    Order      int   // 1,2,3 とは限らない
    TemplateID uuid.UUID
}
```

途中の日を消すと `1,2,5` のように飛ぶ。**`Order` で添字を引かない。**
スライスの並び順で回し、`Order` は表示にだけ使う。

テストで固定してある。

```go
func TestToday_番号が飛んでいても順に回る(t *testing.T)
```
