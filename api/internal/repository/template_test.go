package repository_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func TestTemplateCreate_項目込みで作れる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewTemplate(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)
	tri := firstExercise(t, ex, openapi.Triceps)

	got, err := repo.Create(ctx, repository.TemplateInput{
		Name: "テスト用_Day1 胸",
		Items: []openapi.TemplateItem{
			{ExerciseId: bench.Id, Order: 1, TargetSets: 4, TargetRepsMin: ptr(6), TargetRepsMax: ptr(8), TargetRir: ptr(2)},
			{ExerciseId: tri.Id, Order: 2, TargetSets: 3, TargetRepsMin: ptr(10), TargetRepsMax: ptr(12)},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if got.Name != "テスト用_Day1 胸" {
		t.Errorf("name = %q", got.Name)
	}
	if len(got.Items) != 2 {
		t.Fatalf("項目数 = %d, want 2", len(got.Items))
	}
	if got.Items[0].Order != 1 || got.Items[1].Order != 2 {
		t.Errorf("order が崩れている: %d, %d", got.Items[0].Order, got.Items[1].Order)
	}
	if got.Items[0].TargetRir == nil || *got.Items[0].TargetRir != 2 {
		t.Error("targetRir が保存されていない")
	}
}

func TestTemplateUpdate_項目を差し替える(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewTemplate(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)
	squat := firstExercise(t, ex, openapi.Quads)

	created, err := repo.Create(ctx, repository.TemplateInput{
		Name:  "テスト用_差し替え前",
		Items: []openapi.TemplateItem{{ExerciseId: bench.Id, Order: 1, TargetSets: 4}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Update(ctx, created.Id, repository.TemplateInput{
		Name: "テスト用_差し替え後",
		Items: []openapi.TemplateItem{
			{ExerciseId: squat.Id, Order: 1, TargetSets: 5},
			{ExerciseId: bench.Id, Order: 2, TargetSets: 3},
		},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if got.Name != "テスト用_差し替え後" {
		t.Errorf("name = %q", got.Name)
	}
	// 項目は全入れ替え。古い項目が残ると order が重複する
	if len(got.Items) != 2 {
		t.Fatalf("項目数 = %d, want 2", len(got.Items))
	}
	if got.Items[0].ExerciseId != squat.Id {
		t.Error("1番目が差し替わっていない")
	}
}

func TestTemplateList_論理削除済みは返らない(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewTemplate(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)
	created, err := repo.Create(ctx, repository.TemplateInput{
		Name:  "テスト用_消す",
		Items: []openapi.TemplateItem{{ExerciseId: bench.Id, Order: 1, TargetSets: 4}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.SoftDelete(ctx, created.Id); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	items, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, tpl := range items {
		if tpl.Id == created.Id {
			t.Fatal("論理削除済みが一覧に含まれている")
		}
	}

	if _, err := repo.Get(ctx, created.Id); !repository.IsNotFound(err) {
		t.Errorf("Get err = %v, want ErrNotFound", err)
	}
}

func TestTemplateGet_存在しなければErrNotFound(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewTemplate(testdb.Begin(t))

	if _, err := repo.Get(ctx, testdb.RandomUUID()); !repository.IsNotFound(err) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
