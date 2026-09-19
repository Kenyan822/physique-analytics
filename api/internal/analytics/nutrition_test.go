package analytics_test

import (
	"math"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

// cfg は config.example.json の nutrition と同じ値。
// reference/analysis/analyze.py と突き合わせるための基準になる。
func cfg() analytics.NutritionConfig {
	return analytics.NutritionConfig{
		Cut:                analytics.Macros{ProteinGPerKg: 2.4, FatGPerKg: 0.85},
		DeepCut:            analytics.Macros{ProteinGPerKg: 2.6, FatGPerKg: 0.85},
		Bulk:               analytics.Macros{ProteinGPerKg: 2.2, FatGPerKg: 1.0},
		DeepCutBfThreshold: 13.0,
		CarbMinG:           200,
	}
}

func TestRecommendedIntake(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		tdee, goal, bodyWeight float64
		wantTheoretical        float64
		wantRecommended        float64
		wantFloorHit           bool
	}{
		{
			// 2639 + (-0.5 * 7700 / 7) = 2639 - 550
			name: "減量ペースから理論値を引く",
			tdee: 2639, goal: -0.5, bodyWeight: 75,
			wantTheoretical: 2089, wantRecommended: 2089,
		},
		{
			// 体重75kg の下限は 1800kcal。理論値がそれを下回る
			name: "下限を割ったら下限を採用する",
			tdee: 2400, goal: -1.0, bodyWeight: 75,
			wantTheoretical: 1300, wantRecommended: 1800, wantFloorHit: true,
		},
		{
			// **増量では下限を当てない。** 下限は「削りすぎ」を防ぐためのもの
			name: "増量では下限を当てない",
			tdee: 2600, goal: 0.25, bodyWeight: 60,
			wantTheoretical: 2875, wantRecommended: 2875,
		},
		{
			name: "維持は TDEE そのもの",
			tdee: 2500, goal: 0, bodyWeight: 75,
			wantTheoretical: 2500, wantRecommended: 2500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := analytics.RecommendedIntake(tt.tdee, tt.goal, tt.bodyWeight)

			if math.Abs(got.TheoreticalKcal-tt.wantTheoretical) > 0.5 {
				t.Errorf("TheoreticalKcal = %.1f, want %.1f", got.TheoreticalKcal, tt.wantTheoretical)
			}
			if math.Abs(got.RecommendedKcal-tt.wantRecommended) > 0.5 {
				t.Errorf("RecommendedKcal = %.1f, want %.1f", got.RecommendedKcal, tt.wantRecommended)
			}
			if got.FloorHit != tt.wantFloorHit {
				t.Errorf("FloorHit = %v, want %v", got.FloorHit, tt.wantFloorHit)
			}
		})
	}
}

func TestMacroTargets_フェーズの選び方(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		goal       float64
		bodyfatPct *float64
		want       analytics.NutritionPhase
	}{
		{"増量は bulk", 0.25, ptrF(18), analytics.PhaseBulk},
		{"減量は cut", -0.5, ptrF(18), analytics.PhaseCut},
		// 絞れてくるほどタンパク質を上げる（LBM 保護）
		{"体脂肪率が閾値未満なら deep_cut", -0.5, ptrF(12.5), analytics.PhaseDeepCut},
		{"体脂肪率が未測定なら cut に倒す", -0.5, nil, analytics.PhaseCut},
		{"維持は cut 扱い", 0, ptrF(18), analytics.PhaseCut},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := analytics.MacroTargets(cfg(), 75, tt.bodyfatPct, tt.goal, 2100)
			if got.Phase != tt.want {
				t.Errorf("Phase = %q, want %q", got.Phase, tt.want)
			}
		})
	}
}

func TestMacroTargets_炭水化物は残余で決まる(t *testing.T) {
	t.Parallel()

	// 75kg / cut: P = 180g(720kcal), F = 63.75g(573.75kcal)
	// C = (2100 - 720 - 573.75) / 4 = 201.5625
	got := analytics.MacroTargets(cfg(), 75, ptrF(18), -0.5, 2100)

	if math.Abs(got.ProteinG-180) > 0.01 {
		t.Errorf("ProteinG = %.2f, want 180", got.ProteinG)
	}
	if math.Abs(got.FatG-63.75) > 0.01 {
		t.Errorf("FatG = %.2f, want 63.75", got.FatG)
	}
	if math.Abs(got.CarbG-201.5625) > 0.01 {
		t.Errorf("CarbG = %.4f, want 201.5625", got.CarbG)
	}
	if got.CarbBelowFloor {
		t.Error("CarbBelowFloor = true, want false（下限 200g を上回っている）")
	}
}

func TestMacroTargets_炭水化物が下限を割る(t *testing.T) {
	t.Parallel()

	// 摂取を絞ると、従属変数の炭水化物から先に枯れる。
	// **これは「もっと削れ」ではなく「消費側で赤字を作れ」の合図**
	got := analytics.MacroTargets(cfg(), 75, ptrF(18), -0.5, 1800)

	if !got.CarbBelowFloor {
		t.Errorf("CarbBelowFloor = false, want true（C = %.0fg / 下限 200g）", got.CarbG)
	}
}

func TestMacroTargets_炭水化物が負になっても0で止める(t *testing.T) {
	t.Parallel()

	// P と F だけで摂取枠を超えるケース。負の目標を出しても意味がない
	got := analytics.MacroTargets(cfg(), 100, ptrF(18), -1.0, 1000)

	if got.CarbG != 0 {
		t.Errorf("CarbG = %.1f, want 0", got.CarbG)
	}
	if !got.CarbBelowFloor {
		t.Error("CarbBelowFloor = false, want true")
	}
}

func ptrF(v float64) *float64 { return &v }

func TestKcalFromMacros(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		protein, fat, carb float64
		want               int
	}{
		// Atwater 係数: P 4 / F 9 / C 4
		{"素の計算", 30, 10, 60, 30*4 + 10*9 + 60*4},
		{"全部0", 0, 0, 0, 0},
		{"脂質だけ", 0, 10, 0, 90},
		// **四捨五入する。** 切り捨てだと1日6食で最大6kcal ずれる
		{"端数は四捨五入", 30.1, 10.2, 60.3, 453}, // 120.4 + 91.8 + 241.2 = 453.4
		{"0.5 は切り上げ", 0, 0, 0.125, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := analytics.KcalFromMacros(tt.protein, tt.fat, tt.carb)
			if got != tt.want {
				t.Errorf("KcalFromMacros(%v, %v, %v) = %d, want %d",
					tt.protein, tt.fat, tt.carb, got, tt.want)
			}
		})
	}
}
