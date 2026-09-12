package repository_test

import (
	"math"
	"testing"
	"time"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func planInput() openapi.PlanInput {
	return openapi.PlanInput{
		HeightCm: ptr(float32(175)),
		Phases: []openapi.PlanPhase{
			{Name: "P0 基盤構築", StartsOn: jstDate(2026, 9, 7), EndsOn: jstDate(2026, 9, 30), GoalKgPerWeek: 0},
			{Name: "P1-A カット", StartsOn: jstDate(2026, 10, 1), EndsOn: jstDate(2026, 10, 31), GoalKgPerWeek: -0.76},
		},
		Nutrition: openapi.NutritionSettings{
			Cut:                openapi.MacroRatio{ProteinGPerKg: 2.4, FatGPerKg: 0.85},
			DeepCut:            openapi.MacroRatio{ProteinGPerKg: 2.6, FatGPerKg: 0.85},
			Bulk:               openapi.MacroRatio{ProteinGPerKg: 2.2, FatGPerKg: 1.0},
			DeepCutBfThreshold: 13.0,
			CarbMinG:           200,
		},
		VolumeRanges: []openapi.VolumeRange{
			{MuscleGroup: openapi.Chest, Mev: 10, Mrv: 20},
			{MuscleGroup: openapi.Quads, Mev: 12, Mrv: 22},
		},
	}
}

func TestPutPlan_保存して取得できる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewPlan(testdb.Begin(t))

	if _, err := repo.Put(ctx, planInput()); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.HeightCm == nil || math.Abs(float64(*got.HeightCm)-175) > 0.01 {
		t.Errorf("HeightCm = %v, want 175", got.HeightCm)
	}
	if len(got.Phases) != 2 {
		t.Fatalf("Phases = %d 件, want 2", len(got.Phases))
	}
	// 期間の昇順で返す。フェーズは時系列で読むもの
	if got.Phases[0].Name != "P0 基盤構築" {
		t.Errorf("先頭 = %q, want P0 基盤構築", got.Phases[0].Name)
	}
	if math.Abs(float64(got.Nutrition.Cut.ProteinGPerKg)-2.4) > 0.01 {
		t.Errorf("Cut.ProteinGPerKg = %v, want 2.4", got.Nutrition.Cut.ProteinGPerKg)
	}
}

func TestPutPlan_まるごと置き換える(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewPlan(testdb.Begin(t))

	if _, err := repo.Put(ctx, planInput()); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// **部分更新にしない。** 「消したつもりが残っている」が起きる
	in := planInput()
	in.Phases = in.Phases[:1]
	got, err := repo.Put(ctx, in)
	if err != nil {
		t.Fatalf("Put（2回目）: %v", err)
	}
	if len(got.Phases) != 1 {
		t.Errorf("Phases = %d 件, want 1", len(got.Phases))
	}
}

func TestGetPlan_未設定でも既定のMEVMRVが返る(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewPlan(testdb.Begin(t))

	got, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// マイグレーションで13部位の既定値を入れてある。
	// 空だと全部位が「判定不能」になって分析が成立しない
	if len(got.VolumeRanges) < 13 {
		t.Errorf("VolumeRanges = %d 件, want 13以上", len(got.VolumeRanges))
	}
	if got.Nutrition.CarbMinG == 0 {
		t.Error("栄養パラメータの既定値が入っていない")
	}
}

func TestPutPlan_MEVがMRVを超えたら弾く(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewPlan(testdb.Begin(t))

	in := planInput()
	in.VolumeRanges = []openapi.VolumeRange{{MuscleGroup: openapi.Chest, Mev: 20, Mrv: 10}}

	if _, err := repo.Put(ctx, in); err == nil {
		t.Error("エラーにならない")
	}
}

func TestPlanGoalAt(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewPlan(testdb.Begin(t))

	if _, err := repo.Put(ctx, planInput()); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	tests := []struct {
		name string
		date string
		want float64
		ok   bool
	}{
		{"期間の中", "2026-10-15", -0.76, true},
		{"期間の初日", "2026-10-01", -0.76, true},
		{"別のフェーズ", "2026-09-15", 0, true},
		// **0 を返さない。** 維持期と区別が付かず、停滞判定が変わる
		{"計画の外", "2020-01-01", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			goal, ok := repository.GoalAt(got.Phases, mustParseDate(t, tt.date))
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && math.Abs(goal-tt.want) > 0.001 {
				t.Errorf("goal = %v, want %v", goal, tt.want)
			}
		})
	}
}

func mustParseDate(t *testing.T, s string) time.Time {
	t.Helper()

	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatalf("日付 %q: %v", s, err)
	}

	return d
}

func TestPlanBlocks_保存して取得できる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewPlan(testdb.Begin(t))

	in := []openapi.PlanBlock{
		{Name: "P0 基盤", Months: 1, LbmDeltaKgPerMonth: 0.15, BodyfatPctEnd: 21.2, Focus: ptr("記録習慣の確立")},
		{Name: "P1-A カット", Months: 2, LbmDeltaKgPerMonth: -0.35, BodyfatPctEnd: 18.1},
	}

	got, err := repo.PutBlocks(ctx, in)
	if err != nil {
		t.Fatalf("PutBlocks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ブロック = %d 件, want 2", len(got))
	}
	// 配列の並びが順序になる
	if got[0].Name != "P0 基盤" {
		t.Errorf("先頭 = %q, want P0 基盤", got[0].Name)
	}
	if got[0].Focus == nil || *got[0].Focus != "記録習慣の確立" {
		t.Errorf("Focus = %v", got[0].Focus)
	}
}

func TestPlanBlocks_まるごと置き換える(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewPlan(testdb.Begin(t))

	if _, err := repo.PutBlocks(ctx, []openapi.PlanBlock{
		{Name: "A", Months: 1, BodyfatPctEnd: 20},
		{Name: "B", Months: 1, BodyfatPctEnd: 19},
	}); err != nil {
		t.Fatalf("PutBlocks: %v", err)
	}

	got, err := repo.PutBlocks(ctx, []openapi.PlanBlock{{Name: "C", Months: 1, BodyfatPctEnd: 18}})
	if err != nil {
		t.Fatalf("PutBlocks（2回目）: %v", err)
	}
	if len(got) != 1 || got[0].Name != "C" {
		t.Errorf("got = %+v, want C の1件", got)
	}
}

func TestPlan_起点を保存できる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewPlan(testdb.Begin(t))

	in := planInput()
	in.BaselineWeightKg = ptr(float32(75))
	in.BaselineBodyfatPct = ptr(float32(20))
	in.BaselineMonth = ptr("2026-09")

	if _, err := repo.Put(ctx, in); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.BaselineWeightKg == nil || *got.BaselineWeightKg != 75 {
		t.Errorf("BaselineWeightKg = %v, want 75", got.BaselineWeightKg)
	}
	// DB は date で持つが、返すのは YYYY-MM
	if got.BaselineMonth == nil || *got.BaselineMonth != "2026-09" {
		t.Errorf("BaselineMonth = %v, want 2026-09", got.BaselineMonth)
	}
}
