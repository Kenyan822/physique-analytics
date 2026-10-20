package repository_test

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

// 要件 T-02。入力速度を決める最重要機能なので1リクエストで取れること。
func TestLastPerformance_直近のセッションのセットを返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)

	// 古い日 → 新しい日の順に作る
	for _, d := range []int{5, 12} {
		if _, err := repo.CreateSession(ctx, repository.SessionInput{
			Date: jstDate(2026, 9, d),
			Sets: []repository.SetInput{
				{ExerciseID: bench.Id, SetNo: 1, WeightKg: float32(70 + d), Reps: 8, RIR: ptr(2)},
				{ExerciseID: bench.Id, SetNo: 2, WeightKg: float32(70 + d), Reps: 7, RIR: ptr(1)},
			},
		}); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
	}

	got, err := repo.LastPerformance(ctx, bench.Id, nil)
	if err != nil {
		t.Fatalf("LastPerformance: %v", err)
	}

	if got.Date == nil || got.Date.Format("2006-01-02") != "2026-09-12" {
		t.Fatalf("date = %v, want 2026-09-12（直近のセッション）", got.Date)
	}
	if len(got.Sets) != 2 {
		t.Fatalf("セット数 = %d, want 2", len(got.Sets))
	}
	if got.Sets[0].WeightKg != 82 {
		t.Errorf("weightKg = %v, want 82（9/12 の値）", got.Sets[0].WeightKg)
	}
}

func TestLastPerformance_未実施ならDateがnil(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	// この種目は一度も実施していない
	calf := firstExercise(t, ex, openapi.Calves)

	got, err := repo.LastPerformance(ctx, calf.Id, nil)
	if err != nil {
		t.Fatalf("LastPerformance: %v", err)
	}
	if got.Date != nil {
		t.Errorf("date = %v, want nil", got.Date)
	}
	if len(got.Sets) != 0 {
		t.Errorf("セット数 = %d, want 0", len(got.Sets))
	}
}

// 論理削除したセッションは前回値の対象にしない
func TestLastPerformance_削除済みのセッションは無視する(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)

	old, err := repo.CreateSession(ctx, repository.SessionInput{
		Date: jstDate(2026, 9, 5),
		Sets: []repository.SetInput{{ExerciseID: bench.Id, SetNo: 1, WeightKg: 70, Reps: 8, RIR: ptr(2)}},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	newer, err := repo.CreateSession(ctx, repository.SessionInput{
		Date: jstDate(2026, 9, 12),
		Sets: []repository.SetInput{{ExerciseID: bench.Id, SetNo: 1, WeightKg: 82, Reps: 8, RIR: ptr(2)}},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := repo.DeleteSession(ctx, newer.Id); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	got, err := repo.LastPerformance(ctx, bench.Id, nil)
	if err != nil {
		t.Fatalf("LastPerformance: %v", err)
	}
	if got.Date == nil || got.Date.Format("2006-01-02") != "2026-09-05" {
		t.Errorf("date = %v, want 2026-09-05（削除済みを飛ばす）", got.Date)
	}
	_ = old
}

// before はその日を含まず、それより前で最新のセッションを返す（#257）。
// 今日のセッションを開いている画面が、今日打った値を「前回」と取り違えないため
func TestLastPerformance_beforeより前のセッションを返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)

	// 日ごとに重量を変えて、どの日が返ったかを重量で見分ける
	weights := map[int]float32{5: 55, 12: 62, 20: 70}
	for _, d := range []int{5, 12, 20} {
		if _, err := repo.CreateSession(ctx, repository.SessionInput{
			Date: jstDate(2026, 9, d),
			Sets: []repository.SetInput{{ExerciseID: bench.Id, SetNo: 1, WeightKg: weights[d], Reps: 8, RIR: ptr(2)}},
		}); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
	}

	day := func(d int) *openapi_types.Date { v := jstDate(2026, 9, d); return &v }

	tests := []struct {
		name       string
		before     *openapi_types.Date
		wantDate   string // 空なら nil
		wantWeight float32
	}{
		{"省略すると最新（互換）", nil, "2026-09-20", 70},
		{"その日を含めず前を返す", day(20), "2026-09-12", 62},
		{"その日が無い日でも前を返す", day(15), "2026-09-12", 62},
		{"さらに前", day(12), "2026-09-05", 55},
		{"それより前が無ければ nil", day(5), "", 0},
	}
	for _, tt := range tests {
		got, err := repo.LastPerformance(ctx, bench.Id, tt.before)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if tt.wantDate == "" {
			if got.Date != nil || len(got.Sets) != 0 {
				t.Errorf("%s: date = %v, sets = %d 件, want nil / 0 件", tt.name, got.Date, len(got.Sets))
			}
			continue
		}
		if got.Date == nil || got.Date.Format("2006-01-02") != tt.wantDate {
			t.Errorf("%s: date = %v, want %s", tt.name, got.Date, tt.wantDate)
			continue
		}
		if len(got.Sets) != 1 || got.Sets[0].WeightKg != tt.wantWeight {
			t.Errorf("%s: sets = %+v, want 1 件 %vkg", tt.name, got.Sets, tt.wantWeight)
		}
	}
}
