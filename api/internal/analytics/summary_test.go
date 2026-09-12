package analytics_test

import (
	"math"
	"testing"
	"time"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

var asof = time.Date(2031, 3, 31, 0, 0, 0, 0, time.UTC)

// series は asof から n 日前までの点を作る。fill で各日の値を決める。
func series(n int, fill func(dayAgo int) analytics.DailyPoint) []analytics.DailyPoint {
	out := make([]analytics.DailyPoint, 0, n)
	for i := range n {
		p := fill(i)
		p.Date = asof.AddDate(0, 0, -i)
		out = append(out, p)
	}

	return out
}

func TestBuildStallInput_窓の切り方(t *testing.T) {
	t.Parallel()

	// 30日分。体重は1日 -0.05kg で落ちる
	points := series(30, func(d int) analytics.DailyPoint {
		return analytics.DailyPoint{
			WeightKg: ptrF(75 + 0.05*float64(d)),
			Kcal:     ptrF(2000),
		}
	})

	got := analytics.BuildStallInput(points, asof, -0.35, nil)

	if got.WeightSlopeKgWeek == nil || math.Abs(*got.WeightSlopeKgWeek-(-0.35)) > 1e-6 {
		t.Errorf("WeightSlopeKgWeek = %v, want -0.35", got.WeightSlopeKgWeek)
	}
	// 摂取が一定なので変動係数は0
	if got.KcalCVPct == nil || *got.KcalCVPct != 0 {
		t.Errorf("KcalCVPct = %v, want 0", got.KcalCVPct)
	}
	if got.GoalKgPerWeek != -0.35 {
		t.Errorf("GoalKgPerWeek = %v, want -0.35", got.GoalKgPerWeek)
	}
}

func TestBuildStallInput_7日平均は直近7日だけ(t *testing.T) {
	t.Parallel()

	// 直近7日は疲労度5、それ以前は1。7日平均が5になれば窓が正しい
	points := series(30, func(d int) analytics.DailyPoint {
		f := 1.0
		if d < 7 {
			f = 5.0
		}

		return analytics.DailyPoint{Fatigue: &f}
	})

	got := analytics.BuildStallInput(points, asof, -0.5, nil)

	if got.Fatigue7dAvg == nil || math.Abs(*got.Fatigue7dAvg-5) > 1e-9 {
		t.Errorf("Fatigue7dAvg = %v, want 5", got.Fatigue7dAvg)
	}
}

func TestBuildStallInput_歩数は前週と比べる(t *testing.T) {
	t.Parallel()

	// 直近7日は6000歩、その前の7日は9000歩
	points := series(14, func(d int) analytics.DailyPoint {
		s := 9000.0
		if d < 7 {
			s = 6000.0
		}

		return analytics.DailyPoint{Steps: &s}
	})

	got := analytics.BuildStallInput(points, asof, -0.5, nil)

	if got.Steps7dAvg == nil || *got.Steps7dAvg != 6000 {
		t.Errorf("Steps7dAvg = %v, want 6000", got.Steps7dAvg)
	}
	if got.StepsPrev7dAvg == nil || *got.StepsPrev7dAvg != 9000 {
		t.Errorf("StepsPrev7dAvg = %v, want 9000", got.StepsPrev7dAvg)
	}
}

func TestBuildStallInput_HRVは30日を基準にする(t *testing.T) {
	t.Parallel()

	// 直近7日だけ落ちている。基準（30日平均）より低いことを検知させたい
	points := series(30, func(d int) analytics.DailyPoint {
		v := 70.0
		if d < 7 {
			v = 56.0
		}

		return analytics.DailyPoint{HrvMs: &v}
	})

	got := analytics.BuildStallInput(points, asof, -0.5, nil)

	if got.Hrv7dAvg == nil || *got.Hrv7dAvg != 56 {
		t.Errorf("Hrv7dAvg = %v, want 56", got.Hrv7dAvg)
	}
	// 30日平均 = (7×56 + 23×70) / 30
	want := (7*56.0 + 23*70.0) / 30
	if got.Hrv30dAvg == nil || math.Abs(*got.Hrv30dAvg-want) > 1e-9 {
		t.Errorf("Hrv30dAvg = %v, want %v", got.Hrv30dAvg, want)
	}
}

func TestBuildStallInput_欠損を数える(t *testing.T) {
	t.Parallel()

	// 直近7日のうち3日は体重も摂取も無い
	points := series(7, func(d int) analytics.DailyPoint {
		if d < 3 {
			return analytics.DailyPoint{}
		}

		return analytics.DailyPoint{WeightKg: ptrF(75), Kcal: ptrF(2000)}
	})

	got := analytics.BuildStallInput(points, asof, -0.5, nil)

	if got.MissingWeightDays != 3 {
		t.Errorf("MissingWeightDays = %d, want 3", got.MissingWeightDays)
	}
	if got.MissingKcalDays != 3 {
		t.Errorf("MissingKcalDays = %d, want 3", got.MissingKcalDays)
	}
}

func TestBuildStallInput_記録が無い日は行ごと無くてもよい(t *testing.T) {
	t.Parallel()

	// DB には記録した日の行しか無い。4日分しか無ければ3日欠損
	points := series(4, func(int) analytics.DailyPoint {
		return analytics.DailyPoint{WeightKg: ptrF(75)}
	})

	got := analytics.BuildStallInput(points, asof, -0.5, nil)

	if got.MissingWeightDays != 3 {
		t.Errorf("MissingWeightDays = %d, want 3", got.MissingWeightDays)
	}
}

func TestBuildStallInput_e1RMの傾きはそのまま渡す(t *testing.T) {
	t.Parallel()

	// 種目ごとの回帰は別経路（ExerciseHistory）なので、ここは受け取るだけ
	got := analytics.BuildStallInput(nil, asof, -0.5, ptrF(-0.42))

	if got.E1RMSlopeKgWeek == nil || *got.E1RMSlopeKgWeek != -0.42 {
		t.Errorf("E1RMSlopeKgWeek = %v, want -0.42", got.E1RMSlopeKgWeek)
	}
}

func TestWeeklyStats_体組成の7日平均(t *testing.T) {
	t.Parallel()

	points := series(7, func(int) analytics.DailyPoint {
		return analytics.DailyPoint{WeightKg: ptrF(75.2), BodyfatPct: ptrF(15.4), Kcal: ptrF(2100)}
	})

	got := analytics.WeeklyStats(points, asof)

	if got.WeightKg7dAvg == nil || math.Abs(*got.WeightKg7dAvg-75.2) > 1e-9 {
		t.Errorf("WeightKg7dAvg = %v, want 75.2", got.WeightKg7dAvg)
	}
	if got.BodyfatPct7dAvg == nil || math.Abs(*got.BodyfatPct7dAvg-15.4) > 1e-9 {
		t.Errorf("BodyfatPct7dAvg = %v, want 15.4", got.BodyfatPct7dAvg)
	}
	// TDEE の逆算には21日窓の平均摂取を使う
	if got.MeanKcal21d == nil || math.Abs(*got.MeanKcal21d-2100) > 1e-9 {
		t.Errorf("MeanKcal21d = %v, want 2100", got.MeanKcal21d)
	}
}

func TestInWindow_左端を含めない(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		date time.Time
		want bool
	}{
		{"asof は含む", asof, true},
		{"asof の翌日は含まない", asof.AddDate(0, 0, 1), false},
		{"asof - 6日 は含む", asof.AddDate(0, 0, -6), true},
		// **ここが境界**。含めると点が1つ増えて傾きが変わる
		{"asof - 7日 は含まない", asof.AddDate(0, 0, -7), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := analytics.InWindow(tt.date, asof, 7); got != tt.want {
				t.Errorf("InWindow = %v, want %v", got, tt.want)
			}
		})
	}
}
