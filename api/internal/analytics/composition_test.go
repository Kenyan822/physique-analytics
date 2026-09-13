package analytics_test

import (
	"math"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

func TestLbm(t *testing.T) {
	t.Parallel()

	got := analytics.Lbm(75, 20)
	if math.Abs(got-60) > 1e-9 {
		t.Errorf("Lbm = %v, want 60", got)
	}
}

func TestComposition(t *testing.T) {
	t.Parallel()

	got, ok := analytics.Composition(75, 20, 175)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if math.Abs(got.LbmKg-60) > 1e-9 {
		t.Errorf("LbmKg = %v, want 60", got.LbmKg)
	}
	// NormFFMI と同じ式を二重に持たない
	if math.Abs(got.Ffmi-analytics.NormFFMI(60, 175)) > 1e-9 {
		t.Errorf("Ffmi = %v", got.Ffmi)
	}
}

func TestComposition_出せない入力(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		w, bf, height float64
	}{
		{"体重が0", 0, 20, 175},
		{"身長が0", 75, 20, 0},
		{"体脂肪率が100以上", 75, 100, 175},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, ok := analytics.Composition(tt.w, tt.bf, tt.height); ok {
				t.Error("ok = true, want false")
			}
		})
	}
}

func TestPlanDeviation(t *testing.T) {
	t.Parallel()

	target := analytics.MonthlyTarget{
		Month: "2026-10", Phase: "P1-A", LbmKg: 59.8, BodyfatPct: 18.1, WeightKg: 73.0, Ffmi: 19.8,
	}
	actual := analytics.CompositionResult{LbmKg: 59.0, Ffmi: 19.6}

	got := analytics.PlanDeviation(target, actual, 74.5, 20.8)

	// 実測 − 目標。負なら目標に届いていない
	if math.Abs(got.LbmKg-(-0.8)) > 1e-9 {
		t.Errorf("LbmKg = %v, want -0.8", got.LbmKg)
	}
	if math.Abs(got.WeightKg-1.5) > 1e-9 {
		t.Errorf("WeightKg = %v, want 1.5", got.WeightKg)
	}
	if math.Abs(got.BodyfatPct-2.7) > 1e-9 {
		t.Errorf("BodyfatPct = %v, want 2.7", got.BodyfatPct)
	}
}

func TestPlanDeviation_LBMの遅れを判定する(t *testing.T) {
	t.Parallel()

	target := analytics.MonthlyTarget{LbmKg: 60.0}

	// **LBM が目標を 1kg 以上下回ったら遅れとみなす。**
	// 減量期に LBM が落ちているのは、ペースが速すぎるか回復が足りない
	behind := analytics.PlanDeviation(target, analytics.CompositionResult{LbmKg: 58.5}, 70, 15)
	if !behind.LbmBehind {
		t.Errorf("LbmBehind = false, want true（%.2f kg）", behind.LbmKg)
	}

	ok := analytics.PlanDeviation(target, analytics.CompositionResult{LbmKg: 59.5}, 70, 15)
	if ok.LbmBehind {
		t.Errorf("LbmBehind = true, want false（%.2f kg）", ok.LbmKg)
	}
}

func TestFindMonthlyTarget(t *testing.T) {
	t.Parallel()

	targets := []analytics.MonthlyTarget{
		{Month: "2026-09"}, {Month: "2026-10"}, {Month: "2026-11"},
	}

	got, ok := analytics.FindMonthlyTarget(targets, "2026-10")
	if !ok || got.Month != "2026-10" {
		t.Errorf("got = %+v, ok = %v", got, ok)
	}

	if _, ok := analytics.FindMonthlyTarget(targets, "2027-01"); ok {
		t.Error("計画の外なのに見つかっている")
	}
}
