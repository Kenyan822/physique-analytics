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

## 種目ごとに引かない

今日の想定は6種目前後ある。行ごとに「前回の実施内容」を引くと、
**画面を開くたびに6往復**になる。まとめて1クエリにする。

```sql
select distinct on (s.exercise_id)
    s.exercise_id, ws.date::text, s.weight_kg, s.reps, s.rir
from workout_sets s
    join workout_sessions ws on ws.id = s.session_id and ws.deleted_at is null
where s.exercise_id = any($1::uuid[]) and s.deleted_at is null
order by s.exercise_id, ws.date desc, s.set_no
```

**`distinct on` は Postgres 固有。** `order by` の先頭が `distinct on` の式と
一致している必要がある。そのうえで残りの `order by` が「どの行を残すか」を決める。

ここでは「種目ごとに、最新の日の、1セット目」を1行だけ取る。
ウィンドウ関数（`row_number()`）でも書けるが、`distinct on` の方が短い。

## `any($1)` に `[]uuid.UUID` を渡さない

```go
list := make([]string, 0, len(ids))
for _, id := range ids {
    list = append(list, id.String())
}
// ... where s.exercise_id = any($1::uuid[])
```

本番は `QueryExecModeExec`（Supavisor の transaction mode 向け）で動いており、
**プリペアドを使わないので pgx が要素の型を解決できない**。
文字列にして `::uuid[]` で明示する。食品マスタでも同じ罠を踏んだ。

## 集約はアプリ側で組み立てる

日と種目を別々に引くと、6日ぶんで7往復になる。**join して1回で取り、
`order by` が効いている前提で積む。**

```go
if len(out) == 0 || out[len(out)-1].Order != order {
    out = append(out, RoutineDayRow{Order: order, ...})
}
last := &out[len(out)-1]
last.Items = append(last.Items, it)
```

**`order by rd.day_order, ti.item_order` が崩れると壊れる。**
SQL と Go が暗黙に結びついているので、クエリを触るときは両方見る。
