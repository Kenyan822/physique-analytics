package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Routine はルーティンへのアクセス（要件 T-01 / #232）。
type Routine struct {
	db DBTX
}

// NewRoutine は Routine を作る。
func NewRoutine(db DBTX) *Routine {
	return &Routine{db: db}
}

// RoutineRow は1つのルーティン。
type RoutineRow struct {
	ID   uuid.UUID
	Name string
}

// RoutineDayRow はルーティンの1日ぶん。
type RoutineDayRow struct {
	Order        int
	TemplateID   uuid.UUID
	TemplateName string
	Items        []RoutineItemRow
}

// RoutineItemRow は種目1つ。**前回の実施内容は別で埋める。**
type RoutineItemRow struct {
	ExerciseID    uuid.UUID
	ExerciseName  string
	MuscleGroup   string
	Order         int
	TargetSets    int
	TargetRepsMin *int
	TargetRepsMax *int
	TargetRir     *int
}

// SessionRow は「今日は何日目か」の判断に使うぶんだけ。
type SessionRow struct {
	Date       string
	TemplateID *uuid.UUID
}

// Active は有効なルーティンを返す。**無ければ nil。**
//
// ルーティンが無くても記録はできるので、エラーにしない。
func (r *Routine) Active(ctx context.Context) (*RoutineRow, error) {
	const q = `select id, name from routines
		where is_active and deleted_at is null limit 1`

	var out RoutineRow
	err := r.db.QueryRow(ctx, q).Scan(&out.ID, &out.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ルーティンを引けない: %w", err)
	}

	return &out, nil
}

// Days はルーティンの日と種目をまとめて返す。
//
// **1日ずつ引かない。** 6日ぶんで6回の往復になる。
func (r *Routine) Days(ctx context.Context, routineID uuid.UUID) ([]RoutineDayRow, error) {
	const q = `
		select rd.day_order, t.id, t.name,
			e.id, e.name, e.muscle_group,
			ti.item_order, ti.target_sets,
			ti.target_reps_min, ti.target_reps_max, ti.target_rir
		from routine_days rd
			join templates t on t.id = rd.template_id and t.deleted_at is null
			join template_items ti on ti.template_id = t.id
			join exercises e on e.id = ti.exercise_id
		where rd.routine_id = $1
		order by rd.day_order, ti.item_order`

	rows, err := r.db.Query(ctx, q, routineID)
	if err != nil {
		return nil, fmt.Errorf("ルーティンの日を引けない: %w", err)
	}
	defer rows.Close()

	var out []RoutineDayRow
	for rows.Next() {
		var order int
		var tid uuid.UUID
		var tname string
		var it RoutineItemRow
		if err := rows.Scan(&order, &tid, &tname,
			&it.ExerciseID, &it.ExerciseName, &it.MuscleGroup,
			&it.Order, &it.TargetSets,
			&it.TargetRepsMin, &it.TargetRepsMax, &it.TargetRir); err != nil {
			return nil, fmt.Errorf("ルーティンを読めない: %w", err)
		}

		// 同じ日が続く間は同じ要素に積む。**order by が効いている前提**
		if len(out) == 0 || out[len(out)-1].Order != order {
			out = append(out, RoutineDayRow{Order: order, TemplateID: tid, TemplateName: tname})
		}
		last := &out[len(out)-1]
		last.Items = append(last.Items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ルーティンを読めない: %w", err)
	}

	return out, nil
}

// LastWithSets は「セットがある直近のセッション」を返す。**無ければ nil。**
//
// **セットが無い日は数えない。** 開いただけの日で Day が進むと、
// やっていないのに次に行ってしまう。
//
// before は「その日より前」。今日ぶんは別に見るので含めない。
func (r *Routine) LastWithSets(ctx context.Context, before string) (*SessionRow, error) {
	const q = `
		select s.date::text, s.template_id
		from workout_sessions s
		where s.deleted_at is null
		  and s.date < $1::date
		  and exists (select 1 from workout_sets w where w.session_id = s.id)
		order by s.date desc
		limit 1`

	var out SessionRow
	err := r.db.QueryRow(ctx, q, before).Scan(&out.Date, &out.TemplateID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("直近のセッションを引けない: %w", err)
	}

	return &out, nil
}

// LastRow は種目1つの前回実施内容（1セットぶん）。
type LastRow struct {
	Date     string
	WeightKg float64
	Reps     int
	Rir      *int
}

// LastForExercises は複数種目の前回値をまとめて返す。
//
// **種目ごとに引かない。** 6種目なら6往復になる。画面を開くたびに効く。
//
// 返すのは**その日の1セット目**。行に出すのは目安なので、全セットは要らない。
func (r *Routine) LastForExercises(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]LastRow, error) {
	out := map[uuid.UUID]LastRow{}
	if len(ids) == 0 {
		return out, nil
	}

	// **`any($1)` に []uuid.UUID を渡さない。** 本番は QueryExecModeExec で
	// 動いており、プリペアドを使わないので pgx が要素の型を解決できない
	list := make([]string, 0, len(ids))
	for _, id := range ids {
		list = append(list, id.String())
	}

	const q = `
		select distinct on (s.exercise_id)
			s.exercise_id, ws.date::text, s.weight_kg, s.reps, s.rir
		from workout_sets s
			join workout_sessions ws on ws.id = s.session_id and ws.deleted_at is null
		where s.exercise_id = any($1::uuid[]) and s.deleted_at is null
		order by s.exercise_id, ws.date desc, s.set_no`

	rows, err := r.db.Query(ctx, q, list)
	if err != nil {
		return nil, fmt.Errorf("前回値を引けない: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		var v LastRow
		if err := rows.Scan(&id, &v.Date, &v.WeightKg, &v.Reps, &v.Rir); err != nil {
			return nil, fmt.Errorf("前回値を読めない: %w", err)
		}
		out[id] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("前回値を読めない: %w", err)
	}

	return out, nil
}

// DoneToday はその日に記録した種目を返す。並びと印に使う。
func (r *Routine) DoneToday(ctx context.Context, date string) (map[uuid.UUID]bool, error) {
	const q = `
		select distinct s.exercise_id
		from workout_sets s
			join workout_sessions ws on ws.id = s.session_id and ws.deleted_at is null
		where ws.date = $1::date and s.deleted_at is null`

	rows, err := r.db.Query(ctx, q, date)
	if err != nil {
		return nil, fmt.Errorf("今日の記録を引けない: %w", err)
	}
	defer rows.Close()

	out := map[uuid.UUID]bool{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("今日の記録を読めない: %w", err)
		}
		out[id] = true
	}

	return out, rows.Err()
}

// TodaySession はその日のセッションを返す。**無ければ nil。**
func (r *Routine) TodaySession(ctx context.Context, date string) (*SessionRow, error) {
	const q = `
		select s.date::text, s.template_id
		from workout_sessions s
		where s.deleted_at is null and s.date = $1::date
		  and exists (select 1 from workout_sets w where w.session_id = s.id)
		limit 1`

	var out SessionRow
	err := r.db.QueryRow(ctx, q, date).Scan(&out.Date, &out.TemplateID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("今日のセッションを引けない: %w", err)
	}

	return &out, nil
}
