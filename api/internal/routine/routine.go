// Package routine はルーティンの「今日は何日目か」を決める（要件 T-01 / #232）。
//
// **曜日で決めない。** 日程が乱れてもバランスが崩れないよう、
// やった日だけ進む巡回にしている。
//
// **カウンタを持たない。** 直近のセッションから導くので、
// 記録を消しても整合しない状態が作れない。
package routine

import "github.com/google/uuid"

// Day はルーティンの1日ぶん。テンプレートを指す。
type Day struct {
	// Order は巡回の順。**連番とは限らない**（途中の日を消せる）
	Order      int
	TemplateID uuid.UUID
}

// Session は判断に使うぶんだけのセッション。
type Session struct {
	// Date は JST の日付（ADR-0013）
	Date string
	// TemplateID は nil のことがある（テンプレート無しで自由に記録した日）
	TemplateID *uuid.UUID
}

// Today は今日やる日を返す。**決められなければ nil。**
//
// last は「セットがある直近のセッション」。無ければ nil。
//
//	今日すでに記録がある → その日のまま（入力中に並びが変わらない）
//	無い               → last の次（最後まで行ったら先頭に戻る）
func Today(days []Day, last *Session, today string) *Day {
	if len(days) == 0 {
		return nil
	}

	// **今日の途中で切り替えない。** 打っている最中に並びが変わると混乱する
	if last != nil && last.Date == today {
		if d := find(days, last.TemplateID); d != nil {
			return d
		}
	}

	// 前回が分からなければ先頭から。ルーティン外のテンプレートでやった日も同じ
	if last == nil {
		return &days[0]
	}
	i := indexOf(days, last.TemplateID)
	if i < 0 {
		return &days[0]
	}

	return &days[(i+1)%len(days)]
}

func find(days []Day, id *uuid.UUID) *Day {
	if i := indexOf(days, id); i >= 0 {
		return &days[i]
	}

	return nil
}

func indexOf(days []Day, id *uuid.UUID) int {
	if id == nil {
		return -1
	}
	for i, d := range days {
		if d.TemplateID == *id {
			return i
		}
	}

	return -1
}
