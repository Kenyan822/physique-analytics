package analytics

import "time"

// DayMeal は1日ぶんの食事の判定。
type DayMeal struct {
	// Target はその日に有効だった目標。引けなければ nil
	Target *PFC
	// GoalMet は目標が引けなければ nil（false ではない）
	GoalMet *bool
	// Shortfall は実績 − 目標。負なら足りない。目標が引けなければ nil。
	// 許容幅（±10%）の内外は見ない素の差
	Shortfall *PFC
}

// BuildDayMeal は date（YYYY-MM-DD）の食事を判定する。
//
// targets は StartsOn の昇順。**達成判定は BuildStreak に任せる**（/v1/streaks と
// 同じ定義を二重実装しない）。ここで足すのは、判定に使った目標と、目標との差だけ。
func BuildDayMeal(date string, targets []StreakTarget, consumed PFC) DayMeal {
	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return DayMeal{}
	}

	met := BuildStreak(d, d, targets, map[string]PFC{date: consumed}, nil)[0].MealGoalMet
	if met == nil {
		return DayMeal{}
	}

	// BuildStreak が目標を引けたなら、StartsOn <= date の最新が必ずある
	var target PFC
	for _, t := range targets {
		if t.StartsOn > date {
			break
		}
		target = t.PFC
	}

	return DayMeal{
		Target:  &target,
		GoalMet: met,
		Shortfall: &PFC{
			ProteinG: consumed.ProteinG - target.ProteinG,
			FatG:     consumed.FatG - target.FatG,
			CarbG:    consumed.CarbG - target.CarbG,
		},
	}
}
