package analytics_test

import (
	"math"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

func TestPearson(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		xs, ys []float64
		want   float64
		wantOK bool
	}{
		{"完全な正の相関", []float64{1, 2, 3, 4}, []float64{2, 4, 6, 8}, 1, true},
		{"完全な負の相関", []float64{1, 2, 3, 4}, []float64{8, 6, 4, 2}, -1, true},
		// 手計算: 偏差の積の和が 0 になる並び
		{"無相関", []float64{1, 2, 3, 4}, []float64{3, 1, 4, 2}, 0, true},
		{"強い正の相関", []float64{1, 2, 3, 4}, []float64{1, 3, 2, 4}, 0.8, true},
		// 片方が全部同じ値だと分母が0になる
		{"分散が0なら出せない", []float64{1, 1, 1, 1}, []float64{1, 2, 3, 4}, 0, false},
		{"長さが違えば出せない", []float64{1, 2}, []float64{1, 2, 3}, 0, false},
		{"点が少なければ出せない", []float64{1, 2}, []float64{1, 2}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := analytics.Pearson(tt.xs, tt.ys)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && math.Abs(got-tt.want) > 0.001 {
				t.Errorf("Pearson = %.4f, want %.4f", got, tt.want)
			}
		})
	}
}

func TestCorrelate_サンプルが足りなければ出さない(t *testing.T) {
	t.Parallel()

	// **90日未満の相関は偶然を拾う**（docs/03-分析ロジック.md 分析6）
	xs := make([]float64, 30)
	ys := make([]float64, 30)
	for i := range xs {
		xs[i], ys[i] = float64(i), float64(i)*2
	}

	got := analytics.Correlate("睡眠 → 翌日の e1RM", xs, ys)
	if got.Enough {
		t.Errorf("Enough = true, want false（n=%d）", got.N)
	}
	// 値は出すが「足りない」と分かる形にする。隠すと存在に気づかない
	if got.R == nil {
		t.Error("R = nil。値は出したうえで足りないと示す")
	}
}

func TestCorrelate_十分なサンプル(t *testing.T) {
	t.Parallel()

	xs := make([]float64, analytics.MinCorrelationSamples)
	ys := make([]float64, analytics.MinCorrelationSamples)
	for i := range xs {
		xs[i] = float64(i)
		ys[i] = float64(i)*1.5 + 3
	}

	got := analytics.Correlate("テスト", xs, ys)
	if !got.Enough {
		t.Errorf("Enough = false, want true（n=%d）", got.N)
	}
	if got.R == nil || math.Abs(*got.R-1) > 1e-9 {
		t.Errorf("R = %v, want 1", got.R)
	}
	if got.Strength != analytics.StrengthStrong {
		t.Errorf("Strength = %q, want strong", got.Strength)
	}
}

func TestCorrelationStrength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		r    float64
		want analytics.CorrelationStrength
	}{
		{0.05, analytics.StrengthNone},
		{-0.05, analytics.StrengthNone},
		{0.3, analytics.StrengthWeak},
		{-0.3, analytics.StrengthWeak},
		{0.5, analytics.StrengthModerate},
		{0.8, analytics.StrengthStrong},
		{-0.8, analytics.StrengthStrong},
	}

	for _, tt := range tests {
		t.Run(tt.want.String(), func(t *testing.T) {
			t.Parallel()
			if got := analytics.Strength(tt.r); got != tt.want {
				t.Errorf("Strength(%v) = %q, want %q", tt.r, got, tt.want)
			}
		})
	}
}

func TestLagPairs(t *testing.T) {
	t.Parallel()

	// 睡眠 → **翌日**の e1RM のように、1日ずらして対応づける
	days := []float64{0, 1, 2, 3}
	cause := []float64{7.0, 6.0, 8.0, 5.0}
	effect := []float64{100, 90, 110, 80}

	xs, ys := analytics.LagPairs(days, cause, days, effect, 1)

	// 0日目の睡眠 → 1日目の e1RM、というペアが3つできる
	if len(xs) != 3 || len(ys) != 3 {
		t.Fatalf("ペア = %d / %d, want 3 / 3", len(xs), len(ys))
	}
	if xs[0] != 7.0 || ys[0] != 90 {
		t.Errorf("先頭 = (%v, %v), want (7, 90)", xs[0], ys[0])
	}
}

func TestLagPairs_欠けた日は飛ばす(t *testing.T) {
	t.Parallel()

	// 記録の無い日は行ごと欠ける。翌日が無ければペアにならない
	xs, ys := analytics.LagPairs(
		[]float64{0, 5}, []float64{7, 6},
		[]float64{1, 6}, []float64{100, 90}, 1)

	if len(xs) != 2 {
		t.Fatalf("ペア = %d, want 2", len(xs))
	}
	if ys[0] != 100 || ys[1] != 90 {
		t.Errorf("対応がずれている: %v", ys)
	}
}
