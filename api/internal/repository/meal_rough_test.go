package repository_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func TestMeal_ざっくり入力はroughで保存できる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMeal(testdb.Begin(t))

	rough := openapi.MealSourceRough
	saved, err := repo.Create(ctx, repository.MealInput{
		Date: jstDate(2032, 5, 1), Kcal: ptr(1000),
		ProteinG: ptr(float32(50)), FatG: ptr(float32(33.3)), CarbG: ptr(float32(125)),
		Source: &rough,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if saved.Source != openapi.MealSourceRough {
		t.Errorf("Source = %q, want rough", saved.Source)
	}

	// 手で直しても rough のまま残る（直した値の出どころは「ざっくり入力から始めた記録」）
	updated, err := repo.Update(ctx, saved.Id, repository.MealInput{
		Date: jstDate(2032, 5, 1), Kcal: ptr(1000),
		ProteinG: ptr(float32(80)), FatG: ptr(float32(20)), CarbG: ptr(float32(125)),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Source != openapi.MealSourceRough || updated.ProteinG == nil || *updated.ProteinG != 80 {
		t.Errorf("Update() = source %q, P %v", updated.Source, updated.ProteinG)
	}
}

// **TDEE 推定の母数に rough は入らない。** 推定の入力は daily_metrics.kcal で、
// meals から集計していない。ざっくり入力を足しても推定の入力は変わらない
// （将来 meals から集計するときは、ここを rough 除外の根拠にする）
func TestDailySeries_ざっくり入力はTDEE推定の入力に入らない(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)

	rough := openapi.MealSourceRough
	if _, err := repository.NewMeal(tx).Create(ctx, repository.MealInput{
		Date: jstDate(2032, 6, 1), Kcal: ptr(1500), Source: &rough,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := repository.NewBody(tx).PutDaily(ctx, repository.DailyInput{
		Date: jstDate(2032, 6, 1), WeightKg: ptr(float32(70)),
	}); err != nil {
		t.Fatalf("PutDaily: %v", err)
	}

	got, err := repository.NewAnalysis(tx).DailySeries(ctx, jstDate(2032, 6, 1), jstDate(2032, 6, 1))
	if err != nil {
		t.Fatalf("DailySeries: %v", err)
	}
	if len(got) != 1 || got[0].Kcal != nil {
		t.Errorf("DailySeries = %+v, want kcal が nil（rough は入らない）", got)
	}
}

// ストリーク（#248）は rough を食べた量として数える。数えないと、
// 記録を残した日が「未達」に見えて、ざっくり入力の目的（欠測にしない）に反する
func TestStreak_Inputs_ざっくり入力も食べた量に数える(t *testing.T) {
	t.Parallel()
	tx := testdb.Begin(t)

	rough := openapi.MealSourceRough
	if _, err := repository.NewMeal(tx).Create(t.Context(), repository.MealInput{
		Date: jstDate(2001, 3, 2), Kcal: ptr(1000),
		ProteinG: ptr(float32(50)), FatG: ptr(float32(33.3)), CarbG: ptr(float32(125)),
		Source: &rough,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repository.NewStreak(tx).Inputs(t.Context(), targetDay(t, "2001-03-01"), targetDay(t, "2001-03-03"))
	if err != nil {
		t.Fatalf("Inputs: %v", err)
	}
	if g := got.Consumed["2001-03-02"]; g.ProteinG != 50 || g.CarbG != 125 {
		t.Errorf("Consumed = %+v, want P50 C125", g)
	}
}
