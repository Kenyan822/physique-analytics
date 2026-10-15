package repository_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func TestDailySeries_期間内の記録を返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	body := repository.NewBody(tx)
	repo := repository.NewAnalysis(tx)

	for i, d := range []struct {
		day    int
		weight float64
	}{
		{10, 75.0},
		{11, 74.9},
		{12, 74.8},
	} {
		if _, err := body.PutDaily(ctx, repository.DailyInput{
			Date:     jstDate(2032, 1, d.day),
			WeightKg: ptr(float32(d.weight)),
			Kcal:     ptr(2000 + i),
			Fatigue:  ptr(2),
		}); err != nil {
			t.Fatalf("PutDaily: %v", err)
		}
	}

	got, err := repo.DailySeries(ctx, jstDate(2032, 1, 10), jstDate(2032, 1, 12))
	if err != nil {
		t.Fatalf("DailySeries: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("件数 = %d, want 3", len(got))
	}
	// 集計の窓で使うので、日付の昇順で返す
	if !got[0].Date.Before(got[2].Date) {
		t.Errorf("並び順が昇順でない: %v → %v", got[0].Date, got[2].Date)
	}
	if got[0].WeightKg == nil || *got[0].WeightKg != 75.0 {
		t.Errorf("WeightKg = %v, want 75.0", got[0].WeightKg)
	}
	if got[0].Fatigue == nil || *got[0].Fatigue != 2 {
		t.Errorf("Fatigue = %v, want 2", got[0].Fatigue)
	}
}

func TestDailySeries_未記録の項目はnilで返る(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	body := repository.NewBody(tx)
	repo := repository.NewAnalysis(tx)

	// 体重だけ記録した日。HRV は測っていない
	if _, err := body.PutDaily(ctx, repository.DailyInput{
		Date: jstDate(2032, 2, 1), WeightKg: ptr(float32(75)),
	}); err != nil {
		t.Fatalf("PutDaily: %v", err)
	}

	got, err := repo.DailySeries(ctx, jstDate(2032, 2, 1), jstDate(2032, 2, 1))
	if err != nil {
		t.Fatalf("DailySeries: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("件数 = %d, want 1", len(got))
	}
	// **0 で埋めない。** 0 にすると「HRV 0」として回復不足を誤検知する
	if got[0].HrvMs != nil {
		t.Errorf("HrvMs = %v, want nil", got[0].HrvMs)
	}
}

func TestDailySeries_論理削除した日は含まない(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	body := repository.NewBody(tx)
	repo := repository.NewAnalysis(tx)

	date := jstDate(2032, 3, 1)
	if _, err := body.PutDaily(ctx, repository.DailyInput{Date: date, WeightKg: ptr(float32(75))}); err != nil {
		t.Fatalf("PutDaily: %v", err)
	}
	if err := body.SoftDeleteDaily(ctx, date); err != nil {
		t.Fatalf("SoftDeleteDaily: %v", err)
	}

	got, err := repo.DailySeries(ctx, date, date)
	if err != nil {
		t.Fatalf("DailySeries: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("件数 = %d, want 0", len(got))
	}
}

func TestDailySeries_摂取はmealsから引く(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	body := repository.NewBody(tx)
	meal := repository.NewMeal(tx)
	repo := repository.NewAnalysis(tx)

	date := jstDate(2032, 4, 1)
	if _, err := body.PutDaily(ctx, repository.DailyInput{
		Date: date, WeightKg: ptr(float32(75)),
	}); err != nil {
		t.Fatalf("PutDaily: %v", err)
	}

	// 1日2食。合計 1800kcal / P120 / F50 / C200
	for _, m := range []struct {
		name               string
		kcal               int
		protein, fat, carb float32
	}{
		{"鶏むね", 800, 90, 20, 60},
		{"白米", 1000, 30, 30, 140},
	} {
		if _, err := meal.Create(ctx, repository.MealInput{
			Date: date, Name: ptr(m.name), Kcal: ptr(m.kcal),
			ProteinG: ptr(m.protein), FatG: ptr(m.fat), CarbG: ptr(m.carb),
		}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	got, err := repo.DailySeries(ctx, date, date)
	if err != nil {
		t.Fatalf("DailySeries: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("件数 = %d, want 1", len(got))
	}
	if got[0].Kcal == nil || *got[0].Kcal != 1800 {
		t.Errorf("Kcal = %v, want 1800", got[0].Kcal)
	}
	if got[0].ProteinG == nil || *got[0].ProteinG != 120 {
		t.Errorf("ProteinG = %v, want 120", got[0].ProteinG)
	}
	if got[0].FatG == nil || *got[0].FatG != 50 {
		t.Errorf("FatG = %v, want 50", got[0].FatG)
	}
	if got[0].CarbG == nil || *got[0].CarbG != 200 {
		t.Errorf("CarbG = %v, want 200", got[0].CarbG)
	}
}

func TestDailySeries_食事だけ記録した日も返る(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	meal := repository.NewMeal(tx)
	repo := repository.NewAnalysis(tx)

	// 体重も睡眠も記録していない日。食事だけある
	date := jstDate(2032, 5, 1)
	if _, err := meal.Create(ctx, repository.MealInput{
		Date: date, Name: ptr("そば"), Kcal: ptr(500),
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.DailySeries(ctx, date, date)
	if err != nil {
		t.Fatalf("DailySeries: %v", err)
	}

	// **daily_metrics に行が無くても系列に出る。** 落とすと摂取の記録日数が
	// 足りず、TDEE が出せなくなる
	if len(got) != 1 {
		t.Fatalf("件数 = %d, want 1", len(got))
	}
	if got[0].Kcal == nil || *got[0].Kcal != 500 {
		t.Errorf("Kcal = %v, want 500", got[0].Kcal)
	}
	if got[0].WeightKg != nil {
		t.Errorf("WeightKg = %v, want nil", got[0].WeightKg)
	}
}

func TestDailySeries_論理削除した食事は合計から外れる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	meal := repository.NewMeal(tx)
	repo := repository.NewAnalysis(tx)

	date := jstDate(2032, 6, 1)
	kept, err := meal.Create(ctx, repository.MealInput{
		Date: date, Name: ptr("鶏むね"), Kcal: ptr(400),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	gone, err := meal.Create(ctx, repository.MealInput{
		Date: date, Name: ptr("まちがい"), Kcal: ptr(9000),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := meal.SoftDelete(ctx, gone.Id); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	got, err := repo.DailySeries(ctx, date, date)
	if err != nil {
		t.Fatalf("DailySeries: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("件数 = %d, want 1 (%v)", len(got), kept.Id)
	}
	if got[0].Kcal == nil || *got[0].Kcal != 400 {
		t.Errorf("Kcal = %v, want 400", got[0].Kcal)
	}
}

func TestDailySeries_ざっくり入力が混じった日は摂取ごと外れる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	meal := repository.NewMeal(tx)
	repo := repository.NewAnalysis(tx)

	// 昼は手入力、夜は飲み会でざっくり
	date := jstDate(2032, 7, 1)
	if _, err := meal.Create(ctx, repository.MealInput{
		Date: date, Name: ptr("鶏むね"), Kcal: ptr(600),
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	rough := openapi.MealSourceRough
	if _, err := meal.Create(ctx, repository.MealInput{
		Date: date, Kcal: ptr(1500), Source: &rough,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.DailySeries(ctx, date, date)
	if err != nil {
		t.Fatalf("DailySeries: %v", err)
	}

	// **手入力の 600 だけを1日の摂取として採らない。** 1日分に足りない値を
	// 平均に混ぜると TDEE が低く出て、推奨摂取まで低く出る
	if len(got) != 0 {
		t.Errorf("DailySeries = %+v, want 0 件", got)
	}
}
