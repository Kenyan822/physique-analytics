package repository_test

import (
	"strings"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func TestWeeklyVolume_部位別のセット数を返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewAnalysis(tx)
	w := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)
	squat := firstExercise(t, ex, openapi.Quads)

	sets := []repository.SetInput{}
	for i := 1; i <= 4; i++ {
		sets = append(sets, repository.SetInput{ExerciseID: bench.Id, SetNo: i, WeightKg: 80, Reps: 8, RIR: ptr(2)})
	}
	for i := 5; i <= 7; i++ {
		sets = append(sets, repository.SetInput{ExerciseID: squat.Id, SetNo: i, WeightKg: 100, Reps: 5, RIR: ptr(1)})
	}
	if _, err := w.CreateSession(ctx, repository.SessionInput{Date: jstDate(2031, 3, 10), Sets: sets}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	from, to := jstDate(2031, 3, 9), jstDate(2031, 3, 16)
	got, err := repo.WeeklyVolume(ctx, from, to)
	if err != nil {
		t.Fatalf("WeeklyVolume: %v", err)
	}

	byMuscle := map[openapi.MuscleGroup]repository.MuscleVolume{}
	for _, v := range got {
		byMuscle[v.MuscleGroup] = v
	}

	if chest := byMuscle[openapi.Chest]; chest.Sets != 4 {
		t.Errorf("胸のセット数 = %d, want 4", chest.Sets)
	}
	if quads := byMuscle[openapi.Quads]; quads.Sets != 3 {
		t.Errorf("大腿四頭のセット数 = %d, want 3", quads.Sets)
	}
	// トン数 = Σ(重量 × レップ)
	if chest := byMuscle[openapi.Chest]; chest.TonnageKg != 4*80*8 {
		t.Errorf("胸のトン数 = %v, want %v", chest.TonnageKg, 4*80*8)
	}
}

func TestWeeklyVolume_期間外は数えない(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewAnalysis(tx)
	w := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)
	if _, err := w.CreateSession(ctx, repository.SessionInput{
		Date: jstDate(2031, 4, 1),
		Sets: []repository.SetInput{{ExerciseID: bench.Id, SetNo: 1, WeightKg: 80, Reps: 8, RIR: ptr(2)}},
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	from, to := jstDate(2031, 4, 10), jstDate(2031, 4, 17)
	got, err := repo.WeeklyVolume(ctx, from, to)
	if err != nil {
		t.Fatalf("WeeklyVolume: %v", err)
	}
	for _, v := range got {
		if v.MuscleGroup == openapi.Chest && v.Sets > 0 {
			t.Errorf("期間外のセットが数えられている: %d", v.Sets)
		}
	}
}

func TestExerciseHistory_日付ごとの最大e1RMを返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewAnalysis(tx)
	w := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)
	for i, d := range []int{1, 8, 15} {
		weight := float32(80 + i*2)
		if _, err := w.CreateSession(ctx, repository.SessionInput{
			Date: jstDate(2031, 5, d),
			Sets: []repository.SetInput{
				{ExerciseID: bench.Id, SetNo: 1, WeightKg: 60, Reps: 10, RIR: ptr(3)}, // ウォームアップ
				{ExerciseID: bench.Id, SetNo: 2, WeightKg: weight, Reps: 8, RIR: ptr(2)},
			},
		}); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
	}

	from, to := jstDate(2031, 5, 1), jstDate(2031, 5, 31)
	got, err := repo.ExerciseHistory(ctx, bench.Id, from, to)
	if err != nil {
		t.Fatalf("ExerciseHistory: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("件数 = %d, want 3", len(got))
	}
	// 古い順（傾きの計算に使うため）
	if !got[0].Date.Before(got[2].Date.Time) {
		t.Error("昇順になっていない")
	}
	// 各日の最大 e1RM。E1RM(84, 8, 2) = 112
	if diff := got[2].BestE1RM - 112.0; diff > 0.01 || diff < -0.01 {
		t.Errorf("最終日の e1RM = %v, want 112", got[2].BestE1RM)
	}
}

// RIR 未記録や高レップしかない日は e1RM を出さない
func TestExerciseHistory_計算できない日は含めない(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewAnalysis(tx)
	w := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	side := firstExercise(t, ex, openapi.SideDelts)
	if _, err := w.CreateSession(ctx, repository.SessionInput{
		Date: jstDate(2031, 6, 1),
		Sets: []repository.SetInput{
			{ExerciseID: side.Id, SetNo: 1, WeightKg: 10, Reps: 20, RIR: ptr(0)}, // 高レップ
			{ExerciseID: side.Id, SetNo: 2, WeightKg: 10, Reps: 15},              // RIR 未記録
		},
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	from, to := jstDate(2031, 6, 1), jstDate(2031, 6, 30)
	got, err := repo.ExerciseHistory(ctx, side.Id, from, to)
	if err != nil {
		t.Fatalf("ExerciseHistory: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("件数 = %d, want 0（計算できる日が無い）", len(got))
	}
}

// --- 読み取り専用クエリ ---

func TestQuery_selectを実行できる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewAnalysis(testdb.Begin(t))

	got, err := repo.Query(ctx, "select muscle_group, count(*) as n from exercises group by muscle_group order by n desc", 100)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	if len(got.Columns) != 2 {
		t.Fatalf("列数 = %d, want 2", len(got.Columns))
	}
	if got.Columns[0] != "muscle_group" {
		t.Errorf("列名 = %q", got.Columns[0])
	}
	if len(got.Rows) == 0 {
		t.Error("行が返っていない")
	}
}

// 書き込みを通すと MCP 経由でデータを壊せる
func TestQuery_書き込みを拒否する(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewAnalysis(testdb.Begin(t))

	dangerous := []string{
		"delete from exercises",
		"DELETE FROM exercises",
		"update exercises set name = 'x'",
		"insert into exercises (name, muscle_group) values ('x', '胸')",
		"drop table exercises",
		"truncate exercises",
		"alter table exercises add column x int",
		"create table evil (id int)",
		"grant all on exercises to public",
		// 先頭が select でも、後ろに続けて書ける
		"select 1; delete from exercises",
		// CTE の中に書き込みを隠せる
		"with x as (delete from exercises returning id) select * from x",
	}

	for _, q := range dangerous {
		if _, err := repo.Query(ctx, q, 10); err == nil {
			t.Errorf("通ってしまった: %q", q)
		}
	}
}

func TestQuery_件数を制限する(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewAnalysis(testdb.Begin(t))

	got, err := repo.Query(ctx, "select id from exercises", 5)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got.Rows) > 5 {
		t.Errorf("行数 = %d, limit 5 を超えている", len(got.Rows))
	}
	if !got.Truncated {
		t.Error("打ち切ったことが伝わらない")
	}
}

func TestQuery_エラーは内容を返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewAnalysis(testdb.Begin(t))

	_, err := repo.Query(ctx, "select * from no_such_table", 10)
	if err == nil {
		t.Fatal("エラーを期待したが nil")
	}
	// MCP 経由で Claude が自分で直せるように、DB のエラーはそのまま返す
	if !strings.Contains(err.Error(), "no_such_table") {
		t.Errorf("エラーに原因が含まれていない: %v", err)
	}
}
