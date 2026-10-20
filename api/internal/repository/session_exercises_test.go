package repository_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

// 種目を3つ作る。名前は一意制約があるので UUID を付ける
func seedThreeExercises(t *testing.T, tx pgx.Tx) (a, b, c uuid.UUID) {
	t.Helper()

	n := uuid.NewString()

	return testdb.InsertExercise(t, tx, "種目A-"+n, openapi.Chest),
		testdb.InsertExercise(t, tx, "種目B-"+n, openapi.Chest),
		testdb.InsertExercise(t, tx, "種目C-"+n, openapi.Chest)
}

func newSession(t *testing.T, repo *repository.Workout) uuid.UUID {
	t.Helper()

	s, err := repo.CreateSession(t.Context(), repository.SessionInput{Date: jstDate(2026, 9, 12)})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	return s.Id
}

func exerciseIDs(items []openapi.SessionExercise) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.ExerciseId)
	}

	return ids
}

func sameIDs(got, want []uuid.UUID) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}

	return true
}

// 並び順ごと保存でき、再度置換すると追加・削除・並べ替えが反映される
func TestReplaceSessionExercises_並び順ごと全置換できる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewWorkout(tx)
	a, b, c := seedThreeExercises(t, tx)
	sid := newSession(t, repo)

	tests := []struct {
		name string
		in   []uuid.UUID
	}{
		{"最初の保存", []uuid.UUID{a, b, c}},
		{"並べ替え", []uuid.UUID{c, a, b}},
		{"1つ外す", []uuid.UUID{c, b}},
		{"足す", []uuid.UUID{b, a, c}},
		{"全部外す", []uuid.UUID{}},
	}
	for _, tt := range tests {
		got, err := repo.ReplaceSessionExercises(ctx, sid, tt.in)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if !sameIDs(exerciseIDs(got), tt.in) {
			t.Errorf("%s: 返り値の並び = %v, want %v", tt.name, exerciseIDs(got), tt.in)
		}
		for i, it := range got {
			if it.ItemOrder != i+1 {
				t.Errorf("%s: itemOrder[%d] = %d, want %d", tt.name, i, it.ItemOrder, i+1)
			}
			if it.ExerciseName == "" || it.MuscleGroup == "" {
				t.Errorf("%s: 種目名・部位が空: %+v", tt.name, it)
			}
		}

		// 保存されたものを GET が同じ並びで返す
		s, err := repo.GetSession(ctx, sid)
		if err != nil {
			t.Fatalf("%s: GetSession: %v", tt.name, err)
		}
		if !sameIDs(exerciseIDs(s.Exercises), tt.in) {
			t.Errorf("%s: GetSession の並び = %v, want %v", tt.name, exerciseIDs(s.Exercises), tt.in)
		}
	}
}

// 行が0件のセッションは exercises が空（nil ではなく []）。クライアントがルーティンにフォールバックする
func TestGetSession_種目リストが無ければ空で返る(t *testing.T) {
	t.Parallel()
	tx := testdb.Begin(t)
	repo := repository.NewWorkout(tx)
	sid := newSession(t, repo)

	s, err := repo.GetSession(t.Context(), sid)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if s.Exercises == nil || len(s.Exercises) != 0 {
		t.Errorf("Exercises = %#v, want 空スライス（JSON で [] になる）", s.Exercises)
	}
}

func TestReplaceSessionExercises_仕様に合わない入力は弾く(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewWorkout(tx)
	a, b, c := seedThreeExercises(t, tx)
	sid := newSession(t, repo)

	// a にはセットを記録済み。b は無し
	_, err := repo.CreateSet(ctx, sid, repository.SetInput{ExerciseID: a, SetNo: 1, WeightKg: 60, Reps: 8, RIR: ptr(2)})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}
	initial := []uuid.UUID{a, b}
	if _, err := repo.ReplaceSessionExercises(ctx, sid, initial); err != nil {
		t.Fatalf("初期リスト: %v", err)
	}

	tests := []struct {
		name string
		in   []uuid.UUID
	}{
		{"同じ種目を2回", []uuid.UUID{a, b, b}},
		{"存在しない種目", []uuid.UUID{a, uuid.New()}},
		{"セットがある種目を外す", []uuid.UUID{b, c}},
		{"セットがあるのに全部外す", []uuid.UUID{}},
	}
	for _, tt := range tests {
		_, err := repo.ReplaceSessionExercises(ctx, sid, tt.in)
		if !errors.Is(err, repository.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", tt.name, err)
		}
	}

	// 弾いたあとも元のリストが壊れていない
	s, err := repo.GetSession(ctx, sid)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if !sameIDs(exerciseIDs(s.Exercises), initial) {
		t.Errorf("弾いたのにリストが変わった: %v, want %v", exerciseIDs(s.Exercises), initial)
	}
}

// セットがある種目でも、残したまま並べ替えるのは許す。論理削除済みのセットは「記録」と見なさない
func TestReplaceSessionExercises_セットがある種目の扱い(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewWorkout(tx)
	a, b, _ := seedThreeExercises(t, tx)
	sid := newSession(t, repo)

	set, err := repo.CreateSet(ctx, sid, repository.SetInput{ExerciseID: a, SetNo: 1, WeightKg: 60, Reps: 8, RIR: ptr(2)})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}

	if _, err := repo.ReplaceSessionExercises(ctx, sid, []uuid.UUID{b, a}); err != nil {
		t.Errorf("残したままの並べ替えは通る: %v", err)
	}

	if err := repo.DeleteSet(ctx, set.Id); err != nil {
		t.Fatalf("DeleteSet: %v", err)
	}
	if _, err := repo.ReplaceSessionExercises(ctx, sid, []uuid.UUID{b}); err != nil {
		t.Errorf("削除済みのセットしか無い種目は外せる: %v", err)
	}
}

func TestReplaceSessionExercises_存在しないセッションはErrNotFound(t *testing.T) {
	t.Parallel()
	tx := testdb.Begin(t)
	repo := repository.NewWorkout(tx)
	a, _, _ := seedThreeExercises(t, tx)

	_, err := repo.ReplaceSessionExercises(t.Context(), uuid.New(), []uuid.UUID{a})
	if !repository.IsNotFound(err) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
