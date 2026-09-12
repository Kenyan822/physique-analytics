package analytics_test

import (
	"math"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

func TestMean(t *testing.T) {
	t.Parallel()

	if got := analytics.Mean(nil); got != nil {
		t.Errorf("空の平均 = %v, want nil", got)
	}

	got := analytics.Mean([]float64{2, 4, 6})
	if got == nil || math.Abs(*got-4) > 1e-9 {
		t.Errorf("Mean = %v, want 4", got)
	}
}

func TestCVPct(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		vs   []float64
		want *float64
	}{
		// 標本標準偏差（n-1）で割る。Python の pandas.std() と揃える
		{"ばらつきを百分率で返す", []float64{90, 100, 110}, ptrF(10)},
		{"すべて同じなら0", []float64{100, 100, 100}, ptrF(0)},
		{"2点以下では出さない", []float64{100, 110}, nil},
		{"平均が0なら出さない", []float64{-1, 0, 1}, nil},
		{"空なら出さない", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := analytics.CVPct(tt.vs)

			switch {
			case tt.want == nil && got != nil:
				t.Errorf("CVPct = %v, want nil", *got)
			case tt.want != nil && got == nil:
				t.Errorf("CVPct = nil, want %v", *tt.want)
			case tt.want != nil && math.Abs(*got-*tt.want) > 0.01:
				t.Errorf("CVPct = %v, want %v", *got, *tt.want)
			}
		})
	}
}

func TestSlopePerWeek(t *testing.T) {
	t.Parallel()

	// 1日 -0.1kg で7日 → -0.7kg/週
	days := []float64{0, 1, 2, 3, 4}
	weights := []float64{75.0, 74.9, 74.8, 74.7, 74.6}

	got := analytics.SlopePerWeek(days, weights)
	if got == nil || math.Abs(*got-(-0.7)) > 1e-6 {
		t.Errorf("SlopePerWeek = %v, want -0.7", got)
	}
}

func TestSlopePerWeek_点が少なければ出さない(t *testing.T) {
	t.Parallel()

	// **3点以下では傾きを出さない。** 体重は日々±0.5kg 動くので、
	// 少ない点に回帰をかけるとノイズをトレンドとして読んでしまう
	if got := analytics.SlopePerWeek([]float64{0, 1, 2}, []float64{75, 74.5, 75.2}); got != nil {
		t.Errorf("SlopePerWeek = %v, want nil", *got)
	}
}

func TestMissingDays(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		days    int
		present int
		want    int
	}{
		{"7日中5日あれば2日欠損", 7, 5, 2},
		{"全部あれば0", 7, 7, 0},
		{"記録より日数が少なくても負にしない", 7, 9, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := analytics.MissingDays(tt.days, tt.present); got != tt.want {
				t.Errorf("MissingDays = %d, want %d", got, tt.want)
			}
		})
	}
}
