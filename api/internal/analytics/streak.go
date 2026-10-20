package analytics

import (
	"math"
	"time"
)

// PFC は P・F・C の組。目標にも実績にも使う。
type PFC struct {
	ProteinG, FatG, CarbG float64
}

// StreakTarget は適用開始日つきの目標（手動目標の履歴の1行）。
type StreakTarget struct {
	// StartsOn は YYYY-MM-DD
	StartsOn string
	PFC
}

// StreakDay は日別の達成フラグ。
type StreakDay struct {
	Date string
	// MealGoalMet は目標が引けない日は nil
	MealGoalMet *bool
	Trained     bool
}

// goalTolerance は目標からの許容幅（±10%）。
const goalTolerance = 0.10

// goalEpsilon は境界（ちょうど ±10%）を浮動小数の誤差で外さないための余裕。
// 180*1.1 は 198.00000000000003 になる
const goalEpsilon = 1e-9

// MealGoalMet は actual の P・F・C **すべて**が target の ±10% 以内かを返す。
//
// kcal は見ない。PFC から導けるので、二重に判定すると PFC が合っているのに
// kcal で外れる日が出る。目標が 0 のときは、実績も 0 のときだけ達成。
func MealGoalMet(target, actual PFC) bool {
	return withinGoal(target.ProteinG, actual.ProteinG) &&
		withinGoal(target.FatG, actual.FatG) &&
		withinGoal(target.CarbG, actual.CarbG)
}

func withinGoal(target, actual float64) bool {
	return math.Abs(actual-target) <= target*goalTolerance+goalEpsilon
}

// BuildStreak は from〜to の全日について達成フラグを返す（昇順）。
//
// targets は **StartsOn の昇順**。各日は「StartsOn <= その日」で最新の1件で判定する。
// **いまの目標で過去日を再計算しない** —— 目標を変えるたびにカレンダーが書き換わる。
// どの目標も当てはまらない日（最初の StartsOn より前、履歴が空）は MealGoalMet を nil にする。
// 記録が無い日も行を返す。
func BuildStreak(from, to time.Time, targets []StreakTarget, consumed map[string]PFC, trained map[string]bool) []StreakDay {
	days := int(to.Sub(from).Hours()/24) + 1
	out := make([]StreakDay, 0, max(days, 0))

	cur := -1 // 適用中の目標の添字。-1 は引けない
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		date := d.Format(time.DateOnly)

		// 日付は YYYY-MM-DD なので文字列比較で順序が付く
		for cur+1 < len(targets) && targets[cur+1].StartsOn <= date {
			cur++
		}

		day := StreakDay{Date: date, Trained: trained[date]}
		if cur >= 0 {
			met := MealGoalMet(targets[cur].PFC, consumed[date])
			day.MealGoalMet = &met
		}
		out = append(out, day)
	}

	return out
}
