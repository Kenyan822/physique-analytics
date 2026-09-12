package plan_test

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kenyan822/physique-analytics/api/internal/plan"
)

// example は公開用の設定例。**実データ（private/config.json）は使わない**（ADR-0002）。
func example(t *testing.T) plan.Plan {
	t.Helper()

	p, err := plan.Load(filepath.Join("..", "..", "..", "config.example.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	return p
}

func TestLoad_設定例を読める(t *testing.T) {
	t.Parallel()

	p := example(t)

	if math.Abs(p.HeightCm-175.0) > 1e-9 {
		t.Errorf("HeightCm = %v, want 175", p.HeightCm)
	}
	if p.Nutrition.CarbMinG != 200 {
		t.Errorf("CarbMinG = %v, want 200", p.Nutrition.CarbMinG)
	}
	if math.Abs(p.Nutrition.Cut.ProteinGPerKg-2.4) > 1e-9 {
		t.Errorf("Cut.ProteinGPerKg = %v, want 2.4", p.Nutrition.Cut.ProteinGPerKg)
	}
	if len(p.Phases) == 0 {
		t.Fatal("Phases が空")
	}
}

func TestLoad_無ければエラー(t *testing.T) {
	t.Parallel()

	if _, err := plan.Load(filepath.Join(t.TempDir(), "none.json")); err == nil {
		t.Error("エラーにならない")
	}
}

func TestGoalAt(t *testing.T) {
	t.Parallel()

	p := example(t)

	tests := []struct {
		name string
		date time.Time
		want float64
	}{
		// config.example.json の P0 基盤構築（2026-09-07 〜 09-30）は維持
		{"期間の中", time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), 0.0},
		{"期間の初日", time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC), 0.0},
		// P1-A カット1.0%（2026-10-01 〜 10-31）
		{"次のフェーズ", time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), -0.76},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := p.GoalAt(tt.date)
			if !ok {
				t.Fatalf("該当するフェーズが無い")
			}
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("GoalAt = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGoalAt_計画の外は該当なし(t *testing.T) {
	t.Parallel()

	p := example(t)

	// **0 を返さない。** 維持期と区別が付かず、停滞判定が変わる
	if _, ok := p.GoalAt(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)); ok {
		t.Error("計画開始前なのに該当している")
	}
}

func TestNutritionConfig_analyticsに渡せる形になる(t *testing.T) {
	t.Parallel()

	got := example(t).NutritionConfig()

	if got.DeepCutBfThreshold != 13.0 {
		t.Errorf("DeepCutBfThreshold = %v, want 13", got.DeepCutBfThreshold)
	}
	if got.Bulk.FatGPerKg != 1.0 {
		t.Errorf("Bulk.FatGPerKg = %v, want 1.0", got.Bulk.FatGPerKg)
	}
}

func TestVolumeRange_部位ごとの設定を引く(t *testing.T) {
	t.Parallel()

	p := example(t)

	// 僧帽筋は間接刺激が多いので直接種目の基準が低い
	lo, hi := p.VolumeRange("僧帽筋")
	if lo != 8 || hi != 18 {
		t.Errorf("僧帽筋 = %d-%d, want 8-18", lo, hi)
	}

	// 設定に無い部位は default に落ちる
	dlo, dhi := p.VolumeRange("存在しない部位")
	if dlo != 10 || dhi != 20 {
		t.Errorf("default = %d-%d, want 10-20", dlo, dhi)
	}
}
