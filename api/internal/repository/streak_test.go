package repository_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

// **共有の表なので、他が commit した行が見える。** 実在しない昔の日付で絞り、
// 履歴は消してから始める（Rollback で戻る）
const streakDay = "2001-03-"

func insertStreakMeal(t *testing.T, tx pgx.Tx, date string, p, f, c any, deleted bool) {
	t.Helper()

	deletedAt := "null"
	if deleted {
		deletedAt = "now()"
	}
	if _, err := tx.Exec(t.Context(),
		`insert into meals (date, name, protein_g, fat_g, carb_g, deleted_at)
		 values ($1::date, 'テスト食', $2, $3, $4, `+deletedAt+`)`, date, p, f, c); err != nil {
		t.Fatalf("meals: %v", err)
	}
}

func insertStreakSet(t *testing.T, tx pgx.Tx, date string, exercise uuid.UUID, setDeleted, sessionDeleted bool) {
	t.Helper()

	sessionDeletedAt, setDeletedAt := "null", "null"
	if sessionDeleted {
		sessionDeletedAt = "now()"
	}
	if setDeleted {
		setDeletedAt = "now()"
	}

	var session uuid.UUID
	if err := tx.QueryRow(t.Context(),
		`insert into workout_sessions (date, deleted_at) values ($1::date, `+sessionDeletedAt+`) returning id`,
		date).Scan(&session); err != nil {
		t.Fatalf("workout_sessions: %v", err)
	}
	if _, err := tx.Exec(t.Context(),
		`insert into workout_sets (session_id, exercise_id, set_no, weight_kg, reps, deleted_at)
		 values ($1, $2, 1, 50, 10, `+setDeletedAt+`)`, session, exercise); err != nil {
		t.Fatalf("workout_sets: %v", err)
	}
}

func TestStreak_Inputs_食事は日ごとに合計する(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)

	insertStreakMeal(t, tx, streakDay+"02", 60.5, 20, 100, false)
	insertStreakMeal(t, tx, streakDay+"02", 40, 10.5, 50, false)
	// 未入力（null）は 0 として足す
	insertStreakMeal(t, tx, streakDay+"03", 30, nil, nil, false)
	// 論理削除は数えない
	insertStreakMeal(t, tx, streakDay+"03", 999, 999, 999, true)
	// 範囲外
	insertStreakMeal(t, tx, streakDay+"09", 100, 100, 100, false)

	got, err := repository.NewStreak(tx).Inputs(ctx, day(t, streakDay+"01"), day(t, streakDay+"05"))
	if err != nil {
		t.Fatalf("Inputs: %v", err)
	}

	tests := []struct {
		date    string
		p, f, c float64
	}{
		{streakDay + "02", 100.5, 30.5, 150},
		{streakDay + "03", 30, 0, 0},
	}
	for _, tt := range tests {
		g := got.Consumed[tt.date]
		if g.ProteinG != tt.p || g.FatG != tt.f || g.CarbG != tt.c {
			t.Errorf("Consumed[%s] = %+v, want P%v F%v C%v", tt.date, g, tt.p, tt.f, tt.c)
		}
	}
	if _, ok := got.Consumed[streakDay+"09"]; ok {
		t.Error("範囲外の日が入っている")
	}
}

func TestStreak_Inputs_筋トレはセットが1件でもある日(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	ex := testdb.InsertExercise(t, tx, "ストリーク用種目-"+uuid.NewString(), openapi.Chest)

	insertStreakSet(t, tx, streakDay+"02", ex, false, false)
	// セットだけ消した日・セッションごと消した日は数えない
	insertStreakSet(t, tx, streakDay+"03", ex, true, false)
	insertStreakSet(t, tx, streakDay+"04", ex, false, true)
	// 範囲外
	insertStreakSet(t, tx, streakDay+"09", ex, false, false)

	got, err := repository.NewStreak(tx).Inputs(ctx, day(t, streakDay+"01"), day(t, streakDay+"05"))
	if err != nil {
		t.Fatalf("Inputs: %v", err)
	}

	want := map[string]bool{streakDay + "02": true}
	if len(got.Trained) != len(want) || !got.Trained[streakDay+"02"] {
		t.Errorf("Trained = %v, want %v", got.Trained, want)
	}
}

func TestStreak_Inputs_目標は履歴を開始日の昇順で返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	targets := repository.NewManualTargets(tx)
	if err := targets.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	for _, e := range []struct {
		startsOn string
		protein  float32
	}{{"2001-03-10", 180}, {"2001-03-01", 150}, {"2001-04-01", 200}} {
		if _, err := targets.Add(ctx, targetEntry(t, e.startsOn, e.protein)); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	got, err := repository.NewStreak(tx).Inputs(ctx, day(t, "2001-03-05"), day(t, "2001-03-15"))
	if err != nil {
		t.Fatalf("Inputs: %v", err)
	}

	// **範囲の前に始まった目標も要る**（from の日に効いているのはそれ）。
	// 範囲より後に始まる目標は要らない
	var starts []string
	for _, tg := range got.Targets {
		starts = append(starts, tg.StartsOn)
	}
	want := []string{"2001-03-01", "2001-03-10"}
	if len(starts) != len(want) || starts[0] != want[0] || starts[1] != want[1] {
		t.Fatalf("Targets = %v, want %v", starts, want)
	}
	if got.Targets[0].ProteinG != 150 || got.Targets[1].ProteinG != 180 {
		t.Errorf("Targets = %+v", got.Targets)
	}
}

func TestStreak_Inputs_履歴が空なら目標も空(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	if err := repository.NewManualTargets(tx).Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := repository.NewStreak(tx).Inputs(ctx, day(t, "2001-03-01"), day(t, "2001-03-02"))
	if err != nil {
		t.Fatalf("Inputs: %v", err)
	}
	if len(got.Targets) != 0 {
		t.Errorf("Targets = %+v, want 空", got.Targets)
	}
}
