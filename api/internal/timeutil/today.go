package timeutil

import "time"

// StartOfDay は t を JST で見た日の 0時を返す。
//
// **`Truncate(24 * time.Hour)` を使わない。** Truncate はエポックからの
// 絶対時刻を切るのでロケーションを見ず、JST では9時に丸まる。
// 朝9時前に呼ぶと前日になる（ADR-0013）。
//
// 返す時刻は **UTC の深夜**。`date` 列は日付そのもので時刻もタイムゾーンも
// 持たないため、`+09:00` の深夜を渡すと UTC に直されて 15:00 前日になり、
// 逆方向に1日ずれる。`time.Parse("2006-01-02", s)` が返す形に揃えている。
func StartOfDay(t time.Time) time.Time {
	j := t.In(JST)

	return time.Date(j.Year(), j.Month(), j.Day(), 0, 0, 0, 0, time.UTC)
}

// Today は今日（JST）の 0時を返す。
func Today() time.Time { return StartOfDay(Now()) }
