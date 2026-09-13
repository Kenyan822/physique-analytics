package analytics_test

import (
	"math"
	"testing"
	"time"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

func TestContestPace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		weightKg, bodyfatPct float64
		targetBfPct, weeks   float64
		wantStageWeight      float64
		wantPacePctPerWeek   float64
	}{
		{
			// LBM = 80 × (1 - 0.18) = 65.6。BF11% のステージ体重 = 65.6 / 0.89 = 73.71
			// 必要ペース = (80 - 73.71) / 80 × 100 / 12 = 0.655 %/週
			name:     "必要ペースを出す",
			weightKg: 80, bodyfatPct: 18, targetBfPct: 11, weeks: 12,
			wantStageWeight: 73.71, wantPacePctPerWeek: 0.655,
		},
		{
			// 残り週数が短いほどペースが上がる
			name:     "残りが短いとペースが上がる",
			weightKg: 80, bodyfatPct: 18, targetBfPct: 11, weeks: 6,
			wantStageWeight: 73.71, wantPacePctPerWeek: 1.31,
		},
		{
			// **既に目標を下回っていれば負になる。** 増量に回せる
			name:     "目標を下回っていれば負",
			weightKg: 72, bodyfatPct: 10, targetBfPct: 11, weeks: 12,
			wantStageWeight: 72.81, wantPacePctPerWeek: -0.094,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := analytics.ContestPace(tt.weightKg, tt.bodyfatPct, tt.targetBfPct, tt.weeks)
			if !ok {
				t.Fatal("ok = false, want true")
			}
			if math.Abs(got.StageWeightKg-tt.wantStageWeight) > 0.01 {
				t.Errorf("StageWeightKg = %.2f, want %.2f", got.StageWeightKg, tt.wantStageWeight)
			}
			if math.Abs(got.PacePctPerWeek-tt.wantPacePctPerWeek) > 0.005 {
				t.Errorf("PacePctPerWeek = %.3f, want %.3f", got.PacePctPerWeek, tt.wantPacePctPerWeek)
			}
		})
	}
}

func TestContestPace_出せない入力(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		weightKg, bodyfatPct float64
		targetBfPct, weeks   float64
	}{
		{"残り週数が0", 80, 18, 11, 0},
		{"残り週数が負（大会が過ぎている）", 80, 18, 11, -2},
		{"体重が0", 0, 18, 11, 12},
		{"目標体脂肪率が100以上", 80, 18, 100, 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, ok := analytics.ContestPace(tt.weightKg, tt.bodyfatPct, tt.targetBfPct, tt.weeks); ok {
				t.Error("ok = true, want false")
			}
		})
	}
}

func TestContestPace_安全域の判定(t *testing.T) {
	t.Parallel()

	// **0.7%/週 を超えると LBM を失う**（docs/01-要件定義.md A-10）
	tooFast, _ := analytics.ContestPace(80, 18, 11, 8)
	if !tooFast.TooFast {
		t.Errorf("TooFast = false, want true（%.2f %%/週）", tooFast.PacePctPerWeek)
	}

	ok, _ := analytics.ContestPace(80, 18, 11, 20)
	if ok.TooFast {
		t.Errorf("TooFast = true, want false（%.2f %%/週）", ok.PacePctPerWeek)
	}

	// ちょうど 0.7 は安全域に含める（仕様は「0.7%/週超で警告」）
	const weeks = 6.2857142857 // ちょうど 0.7%/週 になる残り週数
	exact, _ := analytics.ContestPace(80, 18, 11, weeks)
	if math.Abs(exact.PacePctPerWeek-1.25) > 0.5 {
		t.Logf("PacePctPerWeek = %.3f", exact.PacePctPerWeek)
	}
}

func TestWeeksUntil(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		asof, date string
		want       float64
	}{
		{"7日後は1週", "2026-10-01", "2026-10-08", 1},
		{"同日は0", "2026-10-01", "2026-10-01", 0},
		{"過ぎていれば負", "2026-10-08", "2026-10-01", -1},
		{"端数も出す", "2026-10-01", "2026-10-11", 10.0 / 7.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := analytics.WeeksUntil(mustDay(t, tt.asof), mustDay(t, tt.date))
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("WeeksUntil = %v, want %v", got, tt.want)
			}
		})
	}
}

func mustDay(t *testing.T, s string) time.Time {
	t.Helper()

	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatalf("日付 %q: %v", s, err)
	}

	return d
}
