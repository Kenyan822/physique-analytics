package repository_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func TestExerciseList_シードされた種目が全部返る(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewExercise(testdb.Begin(t))

	got, err := repo.List(ctx, repository.ExerciseFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// マイグレーション 000002 で入れた件数
	if len(got) != 49 {
		t.Errorf("件数 = %d, want 49", len(got))
	}
}

func TestExerciseList_部位で絞れる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewExercise(testdb.Begin(t))

	chest := openapi.Chest
	got, err := repo.List(ctx, repository.ExerciseFilter{MuscleGroup: &chest})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(got) != 8 {
		t.Errorf("胸の件数 = %d, want 8", len(got))
	}
	for _, e := range got {
		if e.MuscleGroup != openapi.Chest {
			t.Errorf("%q の部位 = %q, want %q", e.Name, e.MuscleGroup, openapi.Chest)
		}
	}
}

// ADR-0014: 物理削除しない。既定の一覧には出さない
func TestExerciseList_論理削除済みは既定で返らない(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewExercise(tx)

	id := testdb.InsertExercise(t, tx, "テスト用種目_削除", openapi.Chest)
	if err := repo.SoftDelete(ctx, id); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	got, err := repo.List(ctx, repository.ExerciseFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, e := range got {
		if e.Id == id {
			t.Fatal("論理削除済みが既定の一覧に含まれている")
		}
	}

	withDeleted, err := repo.List(ctx, repository.ExerciseFilter{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("List(includeDeleted): %v", err)
	}
	var found *openapi.Exercise
	for i := range withDeleted {
		if withDeleted[i].Id == id {
			found = &withDeleted[i]
			break
		}
	}
	if found == nil {
		t.Fatal("includeDeleted=true でも返らない")
	}
	if found.DeletedAt == nil {
		t.Error("deletedAt が入っていない")
	}
}

func TestExerciseGet_存在する種目を取れる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewExercise(tx)

	all, err := repo.List(ctx, repository.ExerciseFilter{MuscleGroup: ptr(openapi.Chest)})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("種目が1件もない")
	}

	got, err := repo.Get(ctx, all[0].Id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Id != all[0].Id {
		t.Errorf("id = %v, want %v", got.Id, all[0].Id)
	}
	if got.Name != all[0].Name {
		t.Errorf("name = %q, want %q", got.Name, all[0].Name)
	}
}

func TestExerciseGet_存在しなければErrNotFound(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewExercise(testdb.Begin(t))

	// ランダムな UUID。存在しない
	if _, err := repo.Get(ctx, testdb.RandomUUID()); err == nil {
		t.Fatal("エラーを期待したが nil")
	} else if !isNotFound(err) {
		t.Errorf("err = %v, want repository.ErrNotFound", err)
	}
}

func isNotFound(err error) bool {
	return repository.IsNotFound(err)
}

func ptr[T any](v T) *T { return &v }
