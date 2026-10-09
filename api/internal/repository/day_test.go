package repository_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

// **共有の表なので他が commit した行が見える。** 実在しない昔の日付を使い、
// テストごとに別の日にする（daily_metrics は日付が一意）
const dayPrefix = "2001-05-"

func insertDayMeal(t *testing.T, tx pgx.Tx, date string, kcal int, p, f, c float64) {
	t.Helper()

	if _, err := tx.Exec(t.Context(),
		`insert into meals (date, name, kcal, protein_g, fat_g, carb_g)
		 values ($1::date, 'テスト食', $2, $3, $4, $5)`, date, kcal, p, f, c); err != nil {
		t.Fatalf("meals: %v", err)
	}
}

func insertDaySet(t *testing.T, tx pgx.Tx, session uuid.UUID, exercise uuid.UUID, setNo int, weight float64) {
	t.Helper()

	if _, err := tx.Exec(t.Context(),
		`insert into workout_sets (session_id, exercise_id, set_no, weight_kg, reps, rir)
		 values ($1, $2, $3, $4, 8, 2)`, session, exercise, setNo, weight); err != nil {
		t.Fatalf("workout_sets: %v", err)
	}
}

func TestDay_Records_食事と筋トレと体組成を1回で返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	date := dayPrefix + "02"

	// 食事は日の合計。別の日・論理削除は混ざらない
	insertDayMeal(t, tx, date, 700, 60, 20, 80)
	insertDayMeal(t, tx, date, 500, 40.5, 10, 60)
	insertDayMeal(t, tx, dayPrefix+"03", 999, 99, 99, 99)

	// ルーティンの2日目のテンプレートで、2種目を3セット + 1セット
	for _, q := range []string{
		`update routines set is_active = false where is_active`,
	} {
		if _, err := tx.Exec(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	suffix := uuid.NewString()
	var routine, tpl uuid.UUID
	if err := tx.QueryRow(ctx, `insert into routines (name, is_active) values ($1, true) returning id`,
		"日詳細-"+suffix).Scan(&routine); err != nil {
		t.Fatalf("routines: %v", err)
	}
	if err := tx.QueryRow(ctx, `insert into templates (name) values ($1) returning id`,
		"胸-"+suffix).Scan(&tpl); err != nil {
		t.Fatalf("templates: %v", err)
	}
	if _, err := tx.Exec(ctx, `insert into routine_days (routine_id, day_order, template_id) values ($1, 2, $2)`,
		routine, tpl); err != nil {
		t.Fatalf("routine_days: %v", err)
	}

	press := testdb.InsertExercise(t, tx, "日詳細プレス-"+suffix, openapi.Chest)
	fly := testdb.InsertExercise(t, tx, "日詳細フライ-"+suffix, openapi.Chest)
	var session uuid.UUID
	if err := tx.QueryRow(ctx,
		`insert into workout_sessions (date, template_id) values ($1::date, $2) returning id`,
		date, tpl).Scan(&session); err != nil {
		t.Fatalf("workout_sessions: %v", err)
	}
	insertDaySet(t, tx, session, press, 1, 50)
	insertDaySet(t, tx, session, press, 2, 60)
	insertDaySet(t, tx, session, press, 3, 55)
	insertDaySet(t, tx, session, fly, 4, 20)

	if _, err := tx.Exec(ctx,
		`insert into daily_metrics (date, weight_kg, bodyfat_pct) values ($1::date, 70.4, 15.2)`, date); err != nil {
		t.Fatalf("daily_metrics: %v", err)
	}

	got, err := repository.NewDay(tx).Records(ctx, day(t, date))
	if err != nil {
		t.Fatalf("Records: %v", err)
	}

	if got.Kcal != 1200 || got.Consumed.ProteinG != 100.5 || got.Consumed.FatG != 30 || got.Consumed.CarbG != 140 {
		t.Errorf("食事 = %v kcal, %+v", got.Kcal, got.Consumed)
	}

	w := got.Workout
	if w == nil {
		t.Fatal("Workout = nil")
	}
	if w.TemplateName == nil || *w.TemplateName != "胸-"+suffix {
		t.Errorf("TemplateName = %v", w.TemplateName)
	}
	if w.DayOrder == nil || *w.DayOrder != 2 {
		t.Errorf("DayOrder = %v, want 2（有効なルーティンの2日目）", w.DayOrder)
	}
	if len(w.Exercises) != 2 {
		t.Fatalf("種目 = %+v, want 2 件", w.Exercises)
	}
	// 実施した順（最初のセットが早い方）
	if e := w.Exercises[0]; e.Name != "日詳細プレス-"+suffix || e.SetCount != 3 || e.TopWeightKg != 60 {
		t.Errorf("1種目目 = %+v, want 3 セット・最大 60", e)
	}
	if e := w.Exercises[1]; e.Name != "日詳細フライ-"+suffix || e.SetCount != 1 || e.TopWeightKg != 20 {
		t.Errorf("2種目目 = %+v", e)
	}

	b := got.Body
	if b == nil || b.WeightKg == nil || *b.WeightKg != 70.4 || b.BodyFatPct == nil || *b.BodyFatPct != 15.2 {
		t.Errorf("Body = %+v", b)
	}
}

// 記録が無い日も落ちない。食事は 0、筋トレと体組成は nil
func TestDay_Records_記録が無い日(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	if err := repository.NewManualTargets(tx).Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := repository.NewDay(tx).Records(ctx, day(t, dayPrefix+"04"))
	if err != nil {
		t.Fatalf("Records: %v", err)
	}
	if got.Kcal != 0 || got.Consumed.ProteinG != 0 || got.Consumed.FatG != 0 || got.Consumed.CarbG != 0 {
		t.Errorf("食事 = %v kcal, %+v, want 0", got.Kcal, got.Consumed)
	}
	if got.Workout != nil || got.Body != nil || len(got.Targets) != 0 {
		t.Errorf("Workout=%v Body=%v Targets=%v, want すべて空", got.Workout, got.Body, got.Targets)
	}
}

// セットだけ消した日・セッションごと消した日は筋トレなし。体重だけの日も体組成あり
func TestDay_Records_論理削除と部分的な体組成(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	date := dayPrefix + "05"
	ex := testdb.InsertExercise(t, tx, "日詳細削除-"+uuid.NewString(), openapi.Chest)

	var session uuid.UUID
	if err := tx.QueryRow(ctx, `insert into workout_sessions (date) values ($1::date) returning id`, date).Scan(&session); err != nil {
		t.Fatalf("workout_sessions: %v", err)
	}
	insertDaySet(t, tx, session, ex, 1, 40)
	if _, err := tx.Exec(ctx, `update workout_sets set deleted_at = now() where session_id = $1`, session); err != nil {
		t.Fatalf("delete set: %v", err)
	}
	if _, err := tx.Exec(ctx, `insert into daily_metrics (date, weight_kg) values ($1::date, 71.0)`, date); err != nil {
		t.Fatalf("daily_metrics: %v", err)
	}

	got, err := repository.NewDay(tx).Records(ctx, day(t, date))
	if err != nil {
		t.Fatalf("Records: %v", err)
	}
	if got.Workout != nil {
		t.Errorf("Workout = %+v, want nil（セットが全部消えている）", got.Workout)
	}
	if got.Body == nil || got.Body.WeightKg == nil || got.Body.BodyFatPct != nil {
		t.Errorf("Body = %+v, want 体重だけ", got.Body)
	}
}

// 目標は date 以前に始まった履歴を昇順で返す。日の判定は analytics が行う
func TestDay_Records_目標はその日までの履歴(t *testing.T) {
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
	}{{"2001-05-10", 180}, {"2001-05-01", 150}, {"2001-06-01", 200}} {
		if _, err := targets.Add(ctx, targetEntry(t, e.startsOn, e.protein)); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	got, err := repository.NewDay(tx).Records(ctx, day(t, "2001-05-15"))
	if err != nil {
		t.Fatalf("Records: %v", err)
	}
	if len(got.Targets) != 2 || got.Targets[0].StartsOn != "2001-05-01" || got.Targets[1].StartsOn != "2001-05-10" {
		t.Fatalf("Targets = %+v, want 5/1 と 5/10（6/1 は未来）", got.Targets)
	}
	if got.Targets[0].ProteinG != 150 || got.Targets[1].ProteinG != 180 {
		t.Errorf("Targets = %+v", got.Targets)
	}
}
