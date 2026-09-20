package repository_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func TestManualTargets_設定していなければnil(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewManualTargets(testdb.Begin(t))

	got, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Errorf("Get() = %+v, want nil", got)
	}
}

func TestManualTargets_保存して読める(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewManualTargets(testdb.Begin(t))

	in := openapi.ManualTargets{ProteinG: 180, FatG: 70, CarbG: 250}
	saved, err := repo.Put(ctx, in)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	// kcal は列に持たず、読むときに計算する（Atwater 4/9/4）
	if saved.Kcal == nil || *saved.Kcal != 180*4+70*9+250*4 {
		t.Errorf("Kcal = %v, want %d", saved.Kcal, 180*4+70*9+250*4)
	}

	got, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.ProteinG != 180 || got.CarbG != 250 {
		t.Errorf("Get() = %+v", got)
	}
}

func TestManualTargets_上書きできる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewManualTargets(testdb.Begin(t))

	if _, err := repo.Put(ctx, openapi.ManualTargets{ProteinG: 180, FatG: 70, CarbG: 250}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// **単一行。** 2行目を作らず置き換える
	if _, err := repo.Put(ctx, openapi.ManualTargets{ProteinG: 200, FatG: 60, CarbG: 200}); err != nil {
		t.Fatalf("Put 2回目: %v", err)
	}

	got, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.ProteinG != 200 {
		t.Errorf("ProteinG = %v, want 200", got)
	}
}

func TestManualTargets_消すと自動計算に戻る(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewManualTargets(testdb.Begin(t))

	if _, err := repo.Put(ctx, openapi.ManualTargets{ProteinG: 180, FatG: 70, CarbG: 250}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := repo.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Errorf("Get() = %+v, want nil", got)
	}
}

func TestManualTargets_無くても消せる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewManualTargets(testdb.Begin(t))

	// 設定していない状態で消してもエラーにしない（DELETE は冪等）
	if err := repo.Delete(ctx); err != nil {
		t.Errorf("Delete: %v", err)
	}
}
