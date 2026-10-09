package analytics_test

import (
	"math"
	"testing"
	"time"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

func dayTargets() []analytics.StreakTarget {
	return []analytics.StreakTarget{
		{StartsOn: "2026-09-01", PFC: analytics.PFC{ProteinG: 150, FatG: 60, CarbG: 200}},
		{StartsOn: "2026-10-01", PFC: analytics.PFC{ProteinG: 180, FatG: 70, CarbG: 250}},
	}
}

func boolStr(b *bool) string {
	if b == nil {
		return "nil"
	}
	if *b {
		return "true"
	}

	return "false"
}

func TestBuildDayMeal_目標と判定と差(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		date         string
		targets      []analytics.StreakTarget
		consumed     analytics.PFC
		wantTarget   *analytics.PFC
		wantMet      string
		wantShortfal *analytics.PFC
	}{
		{
			name: "達成。差は素の差で出る", date: "2026-10-05", targets: dayTargets(),
			consumed:   analytics.PFC{ProteinG: 175, FatG: 72, CarbG: 240},
			wantTarget: &analytics.PFC{ProteinG: 180, FatG: 70, CarbG: 250}, wantMet: "true",
			wantShortfal: &analytics.PFC{ProteinG: -5, FatG: 2, CarbG: -10},
		},
		{
			name: "未達。どの栄養素がどれだけ足りないか分かる", date: "2026-10-05", targets: dayTargets(),
			consumed:   analytics.PFC{ProteinG: 150, FatG: 70, CarbG: 250},
			wantTarget: &analytics.PFC{ProteinG: 180, FatG: 70, CarbG: 250}, wantMet: "false",
			wantShortfal: &analytics.PFC{ProteinG: -30, FatG: 0, CarbG: 0},
		},
		{
			name: "過去日は当時の目標で判定する", date: "2026-09-20", targets: dayTargets(),
			consumed:   analytics.PFC{ProteinG: 150, FatG: 60, CarbG: 200},
			wantTarget: &analytics.PFC{ProteinG: 150, FatG: 60, CarbG: 200}, wantMet: "true",
			wantShortfal: &analytics.PFC{},
		},
		{
			name: "目標の開始日当日から新しい目標", date: "2026-10-01", targets: dayTargets(),
			consumed:   analytics.PFC{ProteinG: 150, FatG: 60, CarbG: 200},
			wantTarget: &analytics.PFC{ProteinG: 180, FatG: 70, CarbG: 250}, wantMet: "false",
			wantShortfal: &analytics.PFC{ProteinG: -30, FatG: -10, CarbG: -50},
		},
		{
			name: "最初の目標より前は引けない（false ではなく nil）", date: "2026-08-31", targets: dayTargets(),
			consumed: analytics.PFC{ProteinG: 150}, wantMet: "nil",
		},
		{
			name: "履歴が空なら引けない", date: "2026-10-05", targets: nil,
			consumed: analytics.PFC{}, wantMet: "nil",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := analytics.BuildDayMeal(tt.date, tt.targets, tt.consumed)

			if boolStr(got.GoalMet) != tt.wantMet {
				t.Errorf("GoalMet = %s, want %s", boolStr(got.GoalMet), tt.wantMet)
			}
			if (got.Target == nil) != (tt.wantTarget == nil) || (got.Target != nil && *got.Target != *tt.wantTarget) {
				t.Errorf("Target = %v, want %v", got.Target, tt.wantTarget)
			}
			if (got.Shortfall == nil) != (tt.wantShortfal == nil) {
				t.Fatalf("Shortfall = %v, want %v", got.Shortfall, tt.wantShortfal)
			}
			if got.Shortfall != nil {
				d, w := *got.Shortfall, *tt.wantShortfal
				if math.Abs(d.ProteinG-w.ProteinG) > 1e-9 || math.Abs(d.FatG-w.FatG) > 1e-9 || math.Abs(d.CarbG-w.CarbG) > 1e-9 {
					t.Errorf("Shortfall = %+v, want %+v", d, w)
				}
			}
		})
	}
}

// 達成判定は /v1/streaks（BuildStreak）と一致する。境界（ちょうど ±10%）と目標 0 を含む
func TestBuildDayMeal_判定はBuildStreakと一致する(t *testing.T) {
	t.Parallel()

	targets := append(dayTargets(), analytics.StreakTarget{StartsOn: "2026-10-10"}) // 目標 0
	consumed := []analytics.PFC{
		{ProteinG: 198, FatG: 77, CarbG: 275},   // ちょうど +10%
		{ProteinG: 162, FatG: 63, CarbG: 225},   // ちょうど -10%
		{ProteinG: 161.9, FatG: 63, CarbG: 225}, // わずかに外
		{},
		{ProteinG: 1},
	}
	for _, date := range []string{"2026-08-31", "2026-09-30", "2026-10-01", "2026-10-09", "2026-10-10"} {
		d, err := time.Parse(time.DateOnly, date)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range consumed {
			want := analytics.BuildStreak(d, d, targets, map[string]analytics.PFC{date: c}, nil)[0].MealGoalMet
			got := analytics.BuildDayMeal(date, targets, c).GoalMet

			if boolStr(got) != boolStr(want) {
				t.Errorf("%s %+v: GoalMet = %s, BuildStreak = %s", date, c, boolStr(got), boolStr(want))
			}
		}
	}
}
