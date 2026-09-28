package routine_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/Kenyan822/physique-analytics/api/internal/routine"
)

var (
	day1 = uuid.MustParse("11111111-0000-0000-0000-000000000001")
	day2 = uuid.MustParse("11111111-0000-0000-0000-000000000002")
	day3 = uuid.MustParse("11111111-0000-0000-0000-000000000003")
)

// 3日で1周するルーティン
func cycle() []routine.Day {
	return []routine.Day{
		{Order: 1, TemplateID: day1},
		{Order: 2, TemplateID: day2},
		{Order: 3, TemplateID: day3},
	}
}

func TestToday_今日すでに記録があればその日のまま(t *testing.T) {
	t.Parallel()

	// **途中で切り替わらない。** 入力中に並びが変わると混乱する
	got := routine.Today(cycle(), &routine.Session{Date: "2026-09-28", TemplateID: &day2}, "2026-09-28")

	if got == nil || got.Order != 2 {
		t.Errorf("Today = %v, want Day2", got)
	}
}

func TestToday_前回の次に進む(t *testing.T) {
	t.Parallel()

	got := routine.Today(cycle(), &routine.Session{Date: "2026-09-26", TemplateID: &day2}, "2026-09-28")

	if got == nil || got.Order != 3 {
		t.Errorf("Today = %v, want Day3", got)
	}
}

func TestToday_最後まで行ったら1に戻る(t *testing.T) {
	t.Parallel()

	got := routine.Today(cycle(), &routine.Session{Date: "2026-09-26", TemplateID: &day3}, "2026-09-28")

	if got == nil || got.Order != 1 {
		t.Errorf("Today = %v, want Day1", got)
	}
}

// **休んでもずれない。** 曜日固定にしなかった理由がこれ
func TestToday_何日休んでも次の日から(t *testing.T) {
	t.Parallel()

	got := routine.Today(cycle(), &routine.Session{Date: "2026-09-01", TemplateID: &day1}, "2026-09-28")

	if got == nil || got.Order != 2 {
		t.Errorf("Today = %v, want Day2", got)
	}
}

func TestToday_記録が無ければ1日目(t *testing.T) {
	t.Parallel()

	got := routine.Today(cycle(), nil, "2026-09-28")

	if got == nil || got.Order != 1 {
		t.Errorf("Today = %v, want Day1", got)
	}
}

// ルーティンに無いテンプレートでやった日。**次が決められないので1日目に戻す**
func TestToday_ルーティン外のテンプレートなら1日目(t *testing.T) {
	t.Parallel()

	other := uuid.New()
	got := routine.Today(cycle(), &routine.Session{Date: "2026-09-26", TemplateID: &other}, "2026-09-28")

	if got == nil || got.Order != 1 {
		t.Errorf("Today = %v, want Day1", got)
	}
}

// テンプレート無しのセッション（自由に記録した日）
func TestToday_テンプレート無しの記録は数えない(t *testing.T) {
	t.Parallel()

	got := routine.Today(cycle(), &routine.Session{Date: "2026-09-26"}, "2026-09-28")

	if got == nil || got.Order != 1 {
		t.Errorf("Today = %v, want Day1", got)
	}
}

func TestToday_ルーティンが空なら決められない(t *testing.T) {
	t.Parallel()

	if got := routine.Today(nil, nil, "2026-09-28"); got != nil {
		t.Errorf("Today = %v, want nil", got)
	}
}

// 並びが 1,2,3 でないルーティン（途中の日を消した）
func TestToday_番号が飛んでいても順に回る(t *testing.T) {
	t.Parallel()

	days := []routine.Day{
		{Order: 1, TemplateID: day1},
		{Order: 5, TemplateID: day2},
	}

	got := routine.Today(days, &routine.Session{Date: "2026-09-26", TemplateID: &day1}, "2026-09-28")

	if got == nil || got.Order != 5 {
		t.Errorf("Today = %v, want Day5", got)
	}
}
