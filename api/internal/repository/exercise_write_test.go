package repository_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func TestExerciseCreate_作成して取得できる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewExercise(testdb.Begin(t))

	got, err := repo.Create(ctx, repository.ExerciseInput{
		Name:        "テスト用_ケーブルクロスオーバー",
		MuscleGroup: openapi.Chest,
		IsCompound:  ptr(false),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if got.Name != "テスト用_ケーブルクロスオーバー" {
		t.Errorf("name = %q", got.Name)
	}
	if got.MuscleGroup != openapi.Chest {
		t.Errorf("muscleGroup = %q", got.MuscleGroup)
	}
	if got.Id.String() == "00000000-0000-0000-0000-000000000000" {
		t.Error("id が採番されていない")
	}

	again, err := repo.Get(ctx, got.Id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if again.Id != got.Id {
		t.Errorf("取得した id = %v, want %v", again.Id, got.Id)
	}
}

// クライアントが生成した UUID を指定できる（オフライン入力の冪等性のため）
func TestExerciseCreate_id指定で冪等になる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewExercise(testdb.Begin(t))

	id := testdb.RandomUUID()
	in := repository.ExerciseInput{ID: &id, Name: "テスト用_冪等", MuscleGroup: openapi.Lats}

	first, err := repo.Create(ctx, in)
	if err != nil {
		t.Fatalf("1回目: %v", err)
	}
	second, err := repo.Create(ctx, in)
	if err != nil {
		t.Fatalf("2回目（再送）: %v", err)
	}

	if first.Id != id || second.Id != id {
		t.Errorf("id が指定値にならない: %v / %v", first.Id, second.Id)
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Error("再送で作り直されている（冪等でない）")
	}
}

// openapi.yaml: 表記は固定する。ゆれると時系列が分断される
func TestExerciseCreate_同じ名前は弾く(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewExercise(testdb.Begin(t))

	if _, err := repo.Create(ctx, repository.ExerciseInput{
		Name: "ベンチプレス", MuscleGroup: openapi.Chest,
	}); err == nil {
		t.Fatal("エラーを期待したが nil")
	} else if !repository.IsConflict(err) {
		t.Errorf("err = %v, want repository.ErrConflict", err)
	}
}

func TestExerciseUpdate_更新できる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewExercise(tx)

	id := testdb.InsertExercise(t, tx, "テスト用_更新前", openapi.Chest)
	before, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	got, err := repo.Update(ctx, id, repository.ExerciseInput{
		Name: "テスト用_更新後", MuscleGroup: openapi.Triceps, IsCompound: ptr(true),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if got.Name != "テスト用_更新後" {
		t.Errorf("name = %q", got.Name)
	}
	if got.MuscleGroup != openapi.Triceps {
		t.Errorf("muscleGroup = %q", got.MuscleGroup)
	}
	// ADR-0014: updatedAt が競合解決に使われるので、更新のたびに進む必要がある
	if !got.UpdatedAt.After(before.UpdatedAt) {
		t.Errorf("updatedAt が進んでいない: %v → %v", before.UpdatedAt, got.UpdatedAt)
	}
}

func TestExerciseUpdate_存在しなければErrNotFound(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewExercise(testdb.Begin(t))

	_, err := repo.Update(ctx, testdb.RandomUUID(), repository.ExerciseInput{
		Name: "テスト用_無い", MuscleGroup: openapi.Abs,
	})
	if !repository.IsNotFound(err) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestExerciseSoftDelete_二回目はErrNotFound(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewExercise(tx)

	id := testdb.InsertExercise(t, tx, "テスト用_二重削除", openapi.Abs)
	if err := repo.SoftDelete(ctx, id); err != nil {
		t.Fatalf("1回目: %v", err)
	}
	// 既に削除済みと存在しないを区別しない。クライアントにはどちらも 404
	if err := repo.SoftDelete(ctx, id); !repository.IsNotFound(err) {
		t.Errorf("2回目 err = %v, want ErrNotFound", err)
	}
}

// 論理削除した名前は再利用できる（部分ユニークインデックスの確認）
func TestExerciseCreate_削除済みと同じ名前は作れる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewExercise(tx)

	id := testdb.InsertExercise(t, tx, "テスト用_名前再利用", openapi.Glutes)
	if err := repo.SoftDelete(ctx, id); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	got, err := repo.Create(ctx, repository.ExerciseInput{
		Name: "テスト用_名前再利用", MuscleGroup: openapi.Glutes,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.Id == id {
		t.Error("削除済みの行が復活している")
	}
}
