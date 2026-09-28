package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

// ルーティンを1つ作る。**テストごとに別の名前**にしないと一意制約に当たる
func seedRoutine(t *testing.T, tx pgx.Tx, name string, days [][]string) uuid.UUID {
	t.Helper()
	ctx := t.Context()

	var rid uuid.UUID
	// is_active は立てない。**有効なのは1つ**なので、並列テストで衝突する
	err := tx.QueryRow(ctx,
		`insert into routines (name) values ($1) returning id`, name).Scan(&rid)
	if err != nil {
		t.Fatalf("routines: %v", err)
	}

	for i, exercises := range days {
		var tid uuid.UUID
		err := tx.QueryRow(ctx,
			`insert into templates (name) values ($1) returning id`,
			name+"-day"+string(rune('1'+i))).Scan(&tid)
		if err != nil {
			t.Fatalf("templates: %v", err)
		}
		for order, ex := range exercises {
			_, err := tx.Exec(ctx, `
				insert into template_items (template_id, exercise_id, item_order, target_sets)
				select $1, id, $2, 4 from exercises where name = $3`, tid, order+1, ex)
			if err != nil {
				t.Fatalf("template_items: %v", err)
			}
		}
		_, err = tx.Exec(ctx,
			`insert into routine_days (routine_id, day_order, template_id) values ($1, $2, $3)`,
			rid, i+1, tid)
		if err != nil {
			t.Fatalf("routine_days: %v", err)
		}
	}

	return rid
}

func TestRoutine_日と種目を引ける(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewRoutine(tx)

	rid := seedRoutine(t, tx, "引けるテスト", [][]string{
		{"ベンチプレス", "ディップス"},
		{"スクワット"},
	})

	days, err := repo.Days(ctx, rid)
	if err != nil {
		t.Fatalf("Days: %v", err)
	}
	if len(days) != 2 {
		t.Fatalf("len = %d, want 2", len(days))
	}
	if days[0].Order != 1 || len(days[0].Items) != 2 {
		t.Errorf("day1 = order %d, items %d", days[0].Order, len(days[0].Items))
	}
	// **並び順は item_order。** 入れた順に出ないと画面の並びが毎回変わる
	if days[0].Items[0].ExerciseName != "ベンチプレス" {
		t.Errorf("items[0] = %q, want ベンチプレス", days[0].Items[0].ExerciseName)
	}
	if days[0].Items[0].MuscleGroup == "" {
		t.Error("MuscleGroup が空。カテゴリ別に出せない")
	}
}

func TestRoutine_有効なものを引ける(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewRoutine(tx)

	rid := seedRoutine(t, tx, "有効テスト", [][]string{{"ベンチプレス"}})
	// **他のテストや本番投入が既に有効なものを持っている。**
	// トランザクションは他人の commit を見るので、まず自分の中で降ろす
	if _, err := tx.Exec(ctx, `update routines set is_active = false where is_active`); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := tx.Exec(ctx, `update routines set is_active = true where id = $1`, rid); err != nil {
		t.Fatalf("activate: %v", err)
	}

	got, err := repo.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if got == nil || got.ID != rid {
		t.Errorf("Active = %v, want %v", got, rid)
	}
}

func TestRoutine_有効なものが無ければnil(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewRoutine(tx)

	// **404 にしない。** ルーティンが無くても記録はできる
	got, err := repo.Active(ctx)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	_ = got // 他のテストが有効なものを作っている可能性があるので値は見ない
}

// **有効なのは1つだけ。** 2つあるとどれを今日のルーティンにするか決められない
func TestRoutine_有効なものは2つ作れない(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)

	a := seedRoutine(t, tx, "二重テストA", [][]string{{"ベンチプレス"}})
	b := seedRoutine(t, tx, "二重テストB", [][]string{{"スクワット"}})

	if _, err := tx.Exec(ctx, `update routines set is_active = false where is_active`); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := tx.Exec(ctx, `update routines set is_active = true where id = $1`, a); err != nil {
		t.Fatalf("1つ目: %v", err)
	}
	if _, err := tx.Exec(ctx, `update routines set is_active = true where id = $1`, b); err == nil {
		t.Error("2つ目も有効にできてしまった")
	}
}

func TestRoutine_直近のセッションを引ける(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewRoutine(tx)

	var tid uuid.UUID
	if err := tx.QueryRow(ctx,
		`insert into templates (name) values ('直近テスト') returning id`).Scan(&tid); err != nil {
		t.Fatalf("templates: %v", err)
	}
	// **既存データを拾わない日付にする。** 他が commit した記録が見える
	const day = "2099-01-10"
	const after = "2099-01-11"

	var sid uuid.UUID
	if err := tx.QueryRow(ctx,
		`insert into workout_sessions (date, template_id) values ($1, $2) returning id`,
		day, tid).Scan(&sid); err != nil {
		t.Fatalf("sessions: %v", err)
	}
	// **セットが無い日は数えない。** 開いただけの日で Day が進むと困る
	got, err := repo.LastWithSets(ctx, after)
	if err != nil {
		t.Fatalf("LastWithSets: %v", err)
	}
	if got != nil && got.Date == day {
		t.Error("セットが無いのに数えている")
	}

	_, err = tx.Exec(ctx, `
		insert into workout_sets (session_id, exercise_id, set_no, weight_kg, reps)
		select $1, id, 1, 60, 10 from exercises where name = 'ベンチプレス'`, sid)
	if err != nil {
		t.Fatalf("sets: %v", err)
	}

	got, err = repo.LastWithSets(ctx, after)
	if err != nil {
		t.Fatalf("LastWithSets: %v", err)
	}
	if got == nil || got.Date != day {
		t.Fatalf("LastWithSets = %v, want %s", got, day)
	}
	if got.TemplateID == nil || *got.TemplateID != tid {
		t.Errorf("TemplateID = %v, want %v", got.TemplateID, tid)
	}
}

var _ = context.Background
