package repository_test

import (
	"testing"

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
