package repository_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func jstDate(y int, m time.Month, d int) openapi_types.Date {
	return openapi_types.Date{Time: time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}
}

// セッションとセットを一度に作れる。
// オフラインで記録した1回分のトレーニングをまとめて送る経路（openapi.yaml）。
func TestWorkoutCreate_セットを含めて一括登録できる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)

	got, err := repo.CreateSession(ctx, repository.SessionInput{
		Date: jstDate(2026, 9, 12),
		Note: ptr("胸の日"),
		Sets: []repository.SetInput{
			{ExerciseID: bench.Id, SetNo: 1, WeightKg: 80, Reps: 8, RIR: ptr(2)},
			{ExerciseID: bench.Id, SetNo: 2, WeightKg: 80, Reps: 7, RIR: ptr(1)},
			{ExerciseID: bench.Id, SetNo: 3, WeightKg: 75, Reps: 8, RIR: ptr(2)},
		},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if len(got.Sets) != 3 {
		t.Fatalf("セット数 = %d, want 3", len(got.Sets))
	}
	if got.Sets[0].SetNo != 1 || got.Sets[2].SetNo != 3 {
		t.Errorf("setNo の順序が崩れている: %d, %d", got.Sets[0].SetNo, got.Sets[2].SetNo)
	}
	if got.Sets[0].WeightKg != 80 || got.Sets[0].Reps != 8 {
		t.Errorf("1セット目 = %vkg x %v", got.Sets[0].WeightKg, got.Sets[0].Reps)
	}
	if got.Sets[0].Rir == nil || *got.Sets[0].Rir != 2 {
		t.Error("RIR が保存されていない。これが無いと推定1RMを計算できない")
	}
	if got.Note == nil || *got.Note != "胸の日" {
		t.Errorf("note = %v", got.Note)
	}
}

// クライアント生成の UUID で再送しても作り直されない
func TestWorkoutCreate_同じidの再送は冪等(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)
	id := testdb.RandomUUID()
	in := repository.SessionInput{
		ID:   &id,
		Date: jstDate(2026, 9, 12),
		Sets: []repository.SetInput{{ExerciseID: bench.Id, SetNo: 1, WeightKg: 80, Reps: 8}},
	}

	first, err := repo.CreateSession(ctx, in)
	if err != nil {
		t.Fatalf("1回目: %v", err)
	}
	second, existed, err := repo.CreateSessionIdempotent(ctx, in)
	if err != nil {
		t.Fatalf("2回目: %v", err)
	}

	if !existed {
		t.Error("既存と判定されていない（201 と 200 を出し分けられない）")
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Error("再送で作り直されている")
	}
	if len(second.Sets) != 1 {
		t.Errorf("セットが重複している: %d 件", len(second.Sets))
	}
}

func TestWorkoutList_日付で絞れる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)

	// 件数で検証しない。トランザクションで隔離していても、コミット済みの
	// 実データ（手で叩いた記録など）は見えるため。作った id が入るかを見る
	made := map[string]uuid.UUID{}
	for _, d := range []int{10, 11, 12} {
		s, err := repo.CreateSession(ctx, repository.SessionInput{
			Date: jstDate(2031, 9, d),
			Sets: []repository.SetInput{{ExerciseID: bench.Id, SetNo: 1, WeightKg: 80, Reps: 8}},
		})
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		made[fmt.Sprintf("2031-09-%02d", d)] = s.Id
	}

	from := jstDate(2031, 9, 11)
	to := jstDate(2031, 9, 12)
	got, err := repo.ListSessions(ctx, repository.SessionFilter{From: &from, To: &to, Limit: 50})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	found := map[uuid.UUID]bool{}
	for _, s := range got {
		found[s.Id] = true
	}
	if !found[made["2031-09-11"]] || !found[made["2031-09-12"]] {
		t.Error("範囲内のセッションが返っていない")
	}
	if found[made["2031-09-10"]] {
		t.Error("範囲外のセッションが返っている")
	}

	// 新しい順
	for i := 1; i < len(got); i++ {
		if got[i-1].Date.Before(got[i].Date.Time) {
			t.Errorf("降順になっていない: %v の後に %v", got[i-1].Date, got[i].Date)
		}
	}
}

// ADR-0014: サーバ側が新しければ 409。クライアントの古い更新で上書きさせない
func TestWorkoutUpdate_サーバが新しければ競合(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)
	s, err := repo.CreateSession(ctx, repository.SessionInput{
		Date: jstDate(2026, 9, 12),
		Sets: []repository.SetInput{{ExerciseID: bench.Id, SetNo: 1, WeightKg: 80, Reps: 8}},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	stale := s.UpdatedAt.Add(-time.Hour)
	_, err = repo.UpdateSession(ctx, s.Id, repository.SessionUpdate{
		Note: ptr("古い端末からの更新"), ExpectedUpdatedAt: &stale,
	})
	if !repository.IsConflict(err) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}

	// updatedAt を渡さなければ上書きできる（競合検出はクライアントの任意）
	got, err := repo.UpdateSession(ctx, s.Id, repository.SessionUpdate{Note: ptr("上書き")})
	if err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}
	if got.Note == nil || *got.Note != "上書き" {
		t.Errorf("note = %v", got.Note)
	}
}

// セッションを消したら配下のセットも消える（openapi.yaml の deleteWorkoutSession）
func TestWorkoutDelete_配下のセットも論理削除される(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)
	s, err := repo.CreateSession(ctx, repository.SessionInput{
		Date: jstDate(2026, 9, 12),
		Sets: []repository.SetInput{
			{ExerciseID: bench.Id, SetNo: 1, WeightKg: 80, Reps: 8},
			{ExerciseID: bench.Id, SetNo: 2, WeightKg: 80, Reps: 7},
		},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := repo.DeleteSession(ctx, s.Id); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	if _, err := repo.GetSession(ctx, s.Id); !repository.IsNotFound(err) {
		t.Errorf("削除後の取得 err = %v, want ErrNotFound", err)
	}

	var live int
	if err := tx.QueryRow(ctx,
		`select count(*) from workout_sets where session_id = $1 and deleted_at is null`, s.Id,
	).Scan(&live); err != nil {
		t.Fatalf("セット数を数えられない: %v", err)
	}
	if live != 0 {
		t.Errorf("配下のセットが %d 件残っている", live)
	}
}

func TestWorkoutSet_追加と更新と削除ができる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)
	s, err := repo.CreateSession(ctx, repository.SessionInput{Date: jstDate(2026, 9, 12)})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	set, err := repo.CreateSet(ctx, s.Id, repository.SetInput{
		ExerciseID: bench.Id, SetNo: 1, WeightKg: 100, Reps: 5, RIR: ptr(1),
	})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}
	if set.SessionId != s.Id {
		t.Errorf("sessionId = %v, want %v", set.SessionId, s.Id)
	}

	updated, err := repo.UpdateSet(ctx, set.Id, repository.SetInput{
		ExerciseID: bench.Id, SetNo: 1, WeightKg: 102.5, Reps: 5, RIR: ptr(0),
	})
	if err != nil {
		t.Fatalf("UpdateSet: %v", err)
	}
	if updated.WeightKg != 102.5 {
		t.Errorf("weightKg = %v, want 102.5", updated.WeightKg)
	}

	if err := repo.DeleteSet(ctx, set.Id); err != nil {
		t.Fatalf("DeleteSet: %v", err)
	}
	if err := repo.DeleteSet(ctx, set.Id); !repository.IsNotFound(err) {
		t.Errorf("2回目 err = %v, want ErrNotFound", err)
	}
}

// 同じセッション内で setNo は重複できない（マイグレーション 000001 の部分ユニーク）
func TestWorkoutSet_同じsetNoは重複できない(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)
	s, err := repo.CreateSession(ctx, repository.SessionInput{
		Date: jstDate(2026, 9, 12),
		Sets: []repository.SetInput{{ExerciseID: bench.Id, SetNo: 1, WeightKg: 80, Reps: 8}},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	_, err = repo.CreateSet(ctx, s.Id, repository.SetInput{
		ExerciseID: bench.Id, SetNo: 1, WeightKg: 80, Reps: 8,
	})
	if !repository.IsConflict(err) {
		t.Errorf("err = %v, want ErrConflict", err)
	}
}

func firstExercise(t *testing.T, ex *repository.Exercise, mg openapi.MuscleGroup) openapi.Exercise {
	t.Helper()

	got, err := ex.List(t.Context(), repository.ExerciseFilter{MuscleGroup: &mg})
	if err != nil {
		t.Fatalf("種目を引けない: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("%s の種目が無い", mg)
	}

	return got[0]
}
