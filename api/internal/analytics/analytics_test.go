// Package analytics_test は分析ロジックのテスト。
//
// **テストケースは reference/analysis/tests/test_analyze.py から写している。**
// ADR-0011 の受け入れ条件が「Go の出力が Python 実装と一致すること」なので、
// 期待値を独自に立てず、検証基準の期待値をそのまま持ってくる。
package analytics_test

import (
	"math"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

const eps = 0.01

func closeTo(got, want float64) bool { return math.Abs(got-want) <= eps }

// Epley + RIR補正。限界レップ数 r = reps + rir
func TestE1RM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		weight, reps, rir float64
		want              float64
	}{
		// test_analyze.py: assert analyze.e1rm(75, 8, 2) == pytest.approx(100.0)
		{"1RM 100kg相当", 75, 8, 2, 100.0},
		// RIR が違えば同じ重量×レップでも別の意味を持つ
		{"限界8回", 80, 8, 0, 101.33},
		{"限界10回相当", 80, 8, 2, 106.67},
		// 1レップでも RIR 0 なら 1RM そのものにはならない（Epley の性質）
		{"1レップ", 100, 1, 0, 103.33},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := analytics.E1RM(tt.weight, tt.reps, tt.rir)
			if !closeTo(got, tt.want) {
				t.Errorf("E1RM(%v, %v, %v) = %v, want %v", tt.weight, tt.reps, tt.rir, got, tt.want)
			}
		})
	}
}

func TestE1RM_RIRが増えれば推定値も増える(t *testing.T) {
	t.Parallel()

	limit := analytics.E1RM(80, 8, 0)
	spare := analytics.E1RM(80, 8, 2)
	if spare <= limit {
		t.Errorf("RIR 2 (%v) が RIR 0 (%v) 以下になっている", spare, limit)
	}
}

// 限界レップ数が 12 を超えると Epley の推定が過大になるので対象外にする
func TestUsableForE1RM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		reps int
		rir  *int
		want bool
	}{
		{"通常", 8, ptr(2), true},
		{"ちょうど上限", 10, ptr(2), true},
		{"上限超え", 11, ptr(2), false},
		{"高レップ", 15, ptr(0), false},
		{"RIR 未記録", 5, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := analytics.UsableForE1RM(tt.reps, tt.rir); got != tt.want {
				t.Errorf("UsableForE1RM(%d, %v) = %v, want %v", tt.reps, tt.rir, got, tt.want)
			}
		})
	}
}

// 正規化FFMI = LBM/身長m² + 6.1×(1.8 − 身長m)
func TestNormFFMI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		lbm, heightCm float64
		want          float64
	}{
		// 身長 180cm では補正項が 0 になり、素の FFMI と一致する
		{"180cmでは補正が0", 70, 180, 70 / (1.8 * 1.8)},
		{"176.5cm LBM 60", 60, 176.5, 60/(1.765*1.765) + 6.1*(1.8-1.765)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := analytics.NormFFMI(tt.lbm, tt.heightCm)
			if !closeTo(got, tt.want) {
				t.Errorf("NormFFMI(%v, %v) = %v, want %v", tt.lbm, tt.heightCm, got, tt.want)
			}
		})
	}
}

// 身長が低いほど補正が効く（身長差を打ち消すのが正規化FFMI の目的）
func TestNormFFMI_身長が低いほど補正が大きい(t *testing.T) {
	t.Parallel()

	const lbm = 60.0
	short := analytics.NormFFMI(lbm, 165)
	tall := analytics.NormFFMI(lbm, 185)

	rawShort := lbm / (1.65 * 1.65)
	rawTall := lbm / (1.85 * 1.85)

	if (short - rawShort) <= (tall - rawTall) {
		t.Errorf("補正量が逆転している: 165cm で %v, 185cm で %v", short-rawShort, tall-rawTall)
	}
}

// 海軍式 体脂肪率推定（男性）
func TestNavyBodyfat(t *testing.T) {
	t.Parallel()

	got := analytics.NavyBodyfat(86, 39, 175)
	if got < 5 || got > 40 {
		t.Errorf("NavyBodyfat = %v, 妥当な範囲(5-40)を外れている", got)
	}
}

func TestNavyBodyfat_腹囲が増えれば体脂肪率も増える(t *testing.T) {
	t.Parallel()

	lean := analytics.NavyBodyfat(78, 39, 175)
	fat := analytics.NavyBodyfat(92, 39, 175)
	if fat <= lean {
		t.Errorf("腹囲 92cm (%v) が 78cm (%v) 以下になっている", fat, lean)
	}
}

// TDEE = 平均摂取 − 体重変化から逆算した収支。計算式ではなく実測から求める
func TestEstimateTDEE(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		avgIntake      float64
		slopeKgPerWeek float64
		want           float64
	}{
		// 体重が変わっていなければ、摂取がそのまま TDEE
		{"体重維持", 2500, 0, 2500},
		// 週 -0.5kg = 7700*0.5/7 = 550kcal/日の赤字
		{"週0.5kg減", 2000, -0.5, 2550},
		{"週0.5kg増", 3000, 0.5, 2450},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := analytics.EstimateTDEE(tt.avgIntake, tt.slopeKgPerWeek)
			if !closeTo(got, tt.want) {
				t.Errorf("EstimateTDEE(%v, %v) = %v, want %v", tt.avgIntake, tt.slopeKgPerWeek, got, tt.want)
			}
		})
	}
}

// 最小二乗法。体重トレンドと e1RM の傾きの両方で使う
func TestLinearSlope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		xs, ys []float64
		want   float64
		ok     bool
	}{
		{"完全な直線", []float64{0, 1, 2, 3}, []float64{10, 12, 14, 16}, 2, true},
		{"減少", []float64{0, 1, 2}, []float64{10, 9, 8}, -1, true},
		{"平坦", []float64{0, 1, 2}, []float64{5, 5, 5}, 0, true},
		{"点が足りない", []float64{0}, []float64{5}, 0, false},
		{"x が同一（傾きを決められない）", []float64{1, 1, 1}, []float64{1, 2, 3}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := analytics.LinearSlope(tt.xs, tt.ys)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && !closeTo(got, tt.want) {
				t.Errorf("slope = %v, want %v", got, tt.want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
