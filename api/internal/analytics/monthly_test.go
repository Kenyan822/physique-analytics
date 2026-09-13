package analytics_test

import (
	"math"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

// blocks は reference/analysis/make_monthly_plan.py の BLOCKS を模した2ブロック。
func blocks() []analytics.PlanBlock {
	return []analytics.PlanBlock{
		{Name: "P0 基盤", Months: 1, LbmDeltaKgPerMonth: 0.15, BodyfatPctEnd: 21.2},
		{Name: "P1-A カット", Months: 2, LbmDeltaKgPerMonth: -0.35, BodyfatPctEnd: 18.1},
	}
}

func baseline() analytics.Baseline {
	return analytics.Baseline{WeightKg: 75.0, BodyfatPct: 20.0, HeightCm: 175.0}
}

func TestMonthlyTargets_LBMを起点に組み立てる(t *testing.T) {
	t.Parallel()

	got := analytics.MonthlyTargets(baseline(), blocks(), "2026-09")

	if len(got) != 3 {
		t.Fatalf("月数 = %d, want 3", len(got))
	}

	// 起点 LBM = 75 × (1 - 0.20) = 60.0
	// 1ヶ月目: LBM 60.15 / BF 21.2 → 体重 = 60.15 / (1 - 0.212) = 76.33
	if math.Abs(got[0].LbmKg-60.15) > 0.01 {
		t.Errorf("1ヶ月目 LBM = %.2f, want 60.15", got[0].LbmKg)
	}
	if math.Abs(got[0].BodyfatPct-21.2) > 0.01 {
		t.Errorf("1ヶ月目 BF = %.2f, want 21.2", got[0].BodyfatPct)
	}
	if math.Abs(got[0].WeightKg-76.33) > 0.01 {
		t.Errorf("1ヶ月目 体重 = %.2f, want 76.33", got[0].WeightKg)
	}
	if got[0].Month != "2026-09" {
		t.Errorf("Month = %q, want 2026-09", got[0].Month)
	}
}

func TestMonthlyTargets_体脂肪率はブロック内で等分する(t *testing.T) {
	t.Parallel()

	got := analytics.MonthlyTargets(baseline(), blocks(), "2026-09")

	// P1-A は2ヶ月で 21.2 → 18.1。1ヶ月あたり -1.55
	if math.Abs(got[1].BodyfatPct-19.65) > 0.01 {
		t.Errorf("2ヶ月目 BF = %.2f, want 19.65", got[1].BodyfatPct)
	}
	if math.Abs(got[2].BodyfatPct-18.1) > 0.01 {
		t.Errorf("3ヶ月目 BF = %.2f, want 18.1", got[2].BodyfatPct)
	}
}

func TestMonthlyTargets_月ラベルが繰り上がる(t *testing.T) {
	t.Parallel()

	bs := []analytics.PlanBlock{{Name: "長いブロック", Months: 5, LbmDeltaKgPerMonth: 0, BodyfatPctEnd: 20}}
	got := analytics.MonthlyTargets(baseline(), bs, "2026-11")

	want := []string{"2026-11", "2026-12", "2027-01", "2027-02", "2027-03"}
	for i, w := range want {
		if got[i].Month != w {
			t.Errorf("[%d] = %q, want %q", i, got[i].Month, w)
		}
	}
}

func TestMonthlyTargets_FFMIを出す(t *testing.T) {
	t.Parallel()

	got := analytics.MonthlyTargets(baseline(), blocks(), "2026-09")

	// NormFFMI(60.15, 175) と一致すること。式を二重に持たない
	want := analytics.NormFFMI(60.15, 175.0)
	if math.Abs(got[0].Ffmi-want) > 0.01 {
		t.Errorf("FFMI = %.2f, want %.2f", got[0].Ffmi, want)
	}
}

func TestMonthlyTargets_実測を起点に引き直せる(t *testing.T) {
	t.Parallel()

	// **要件 P-03。** 計画どおりに進まなかったときは、実測から引き直す。
	// 起点が変われば以降の全ての月がずれる
	planned := analytics.MonthlyTargets(baseline(), blocks(), "2026-09")

	measured := analytics.Baseline{WeightKg: 73.0, BodyfatPct: 19.0, HeightCm: 175.0}
	redone := analytics.MonthlyTargets(measured, blocks(), "2026-09")

	if math.Abs(redone[0].LbmKg-planned[0].LbmKg) < 0.5 {
		t.Errorf("引き直しで LBM が変わっていない: %.2f / %.2f", redone[0].LbmKg, planned[0].LbmKg)
	}
	// 起点 LBM = 73 × 0.81 = 59.13 → 1ヶ月目 59.28
	if math.Abs(redone[0].LbmKg-59.28) > 0.01 {
		t.Errorf("LBM = %.2f, want 59.28", redone[0].LbmKg)
	}
}

func TestMonthlyTargets_空のブロック(t *testing.T) {
	t.Parallel()

	if got := analytics.MonthlyTargets(baseline(), nil, "2026-09"); len(got) != 0 {
		t.Errorf("月数 = %d, want 0", len(got))
	}
}

func TestMonthlyTargets_月数が0のブロックは飛ばす(t *testing.T) {
	t.Parallel()

	bs := []analytics.PlanBlock{
		{Name: "空", Months: 0, BodyfatPctEnd: 20},
		{Name: "1ヶ月", Months: 1, BodyfatPctEnd: 19},
	}
	got := analytics.MonthlyTargets(baseline(), bs, "2026-09")

	if len(got) != 1 || got[0].Phase != "1ヶ月" {
		t.Errorf("got = %+v, want 1ヶ月 の1件", got)
	}
}

func TestNextMonth(t *testing.T) {
	t.Parallel()

	tests := []struct{ in, want string }{
		{"2026-09", "2026-10"},
		{"2026-12", "2027-01"},
		{"2027-01", "2027-02"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			if got := analytics.NextMonth(tt.in); got != tt.want {
				t.Errorf("NextMonth(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
