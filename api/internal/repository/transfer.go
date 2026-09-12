package repository

import (
	"context"
	"fmt"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/internal/csvio"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// Transfer は CSV の入出力（要件 I-01 / ADR-0011）。
type Transfer struct {
	db DBTX
}

// NewTransfer は Transfer を作る。
func NewTransfer(db DBTX) *Transfer {
	return &Transfer{db: db}
}

// OnDuplicate は同じ日付の行が既にあるときの扱い。
type OnDuplicate string

const (
	// OnDuplicateSkip は既存を残す（既定）。取り込みで既存を壊さない
	OnDuplicateSkip OnDuplicate = "skip"
	// OnDuplicateOverwrite は上書きする
	OnDuplicateOverwrite OnDuplicate = "overwrite"
)

// ImportResult は取り込みの結果。
type ImportResult struct {
	Imported int
	Skipped  int
	Errors   []csvio.RowError
}

// --- daily ---

// ExportDaily は期間内の日次記録を CSV の行として返す。
func (r *Transfer) ExportDaily(ctx context.Context, from, to *openapi_types.Date) ([]csvio.DailyRow, error) {
	const q = `
		select date, weight_kg, bodyfat_pct, kcal, protein_g, fat_g, carb_g,
		       sleep_h, steps, fatigue, hrv_ms, resting_hr, deep_sleep_min, note
		from daily_metrics
		where deleted_at is null
		  and ($1::date is null or date >= $1::date)
		  and ($2::date is null or date <= $2::date)
		order by date`

	rows, err := r.db.Query(ctx, q, dateOrNil(from), dateOrNil(to))
	if err != nil {
		return nil, fmt.Errorf("日次記録を引けない: %w", err)
	}
	defer rows.Close()

	out := make([]csvio.DailyRow, 0, 256)
	for rows.Next() {
		var v csvio.DailyRow
		err := rows.Scan(&v.Date, &v.WeightKg, &v.BodyfatPct, &v.Kcal, &v.ProteinG, &v.FatG, &v.CarbG,
			&v.SleepH, &v.Steps, &v.Fatigue, &v.HrvMs, &v.RestingHr, &v.DeepSleepMin, &v.Note)
		if err != nil {
			return nil, fmt.Errorf("日次記録を読めない: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("日次記録を読めない: %w", err)
	}

	return out, nil
}

// ImportDaily は日次記録を取り込む。
func (r *Transfer) ImportDaily(ctx context.Context, rows []csvio.DailyRow, on OnDuplicate) (ImportResult, error) {
	// 同じ日付が既にあるときの扱いを SQL 側で出し分ける。
	// skip は do nothing、overwrite は do update
	const qSkip = `
		insert into daily_metrics
			(date, weight_kg, bodyfat_pct, kcal, protein_g, fat_g, carb_g,
			 sleep_h, steps, fatigue, hrv_ms, resting_hr, deep_sleep_min, note)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		on conflict (date) where deleted_at is null do nothing`

	const qOverwrite = `
		insert into daily_metrics
			(date, weight_kg, bodyfat_pct, kcal, protein_g, fat_g, carb_g,
			 sleep_h, steps, fatigue, hrv_ms, resting_hr, deep_sleep_min, note)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		on conflict (date) where deleted_at is null do update set
			weight_kg = excluded.weight_kg, bodyfat_pct = excluded.bodyfat_pct,
			kcal = excluded.kcal, protein_g = excluded.protein_g,
			fat_g = excluded.fat_g, carb_g = excluded.carb_g,
			sleep_h = excluded.sleep_h, steps = excluded.steps,
			fatigue = excluded.fatigue, hrv_ms = excluded.hrv_ms,
			resting_hr = excluded.resting_hr, deep_sleep_min = excluded.deep_sleep_min,
			note = excluded.note, updated_at = now()`

	q := qSkip
	if on == OnDuplicateOverwrite {
		q = qOverwrite
	}

	out := ImportResult{Errors: []csvio.RowError{}}
	for i, v := range rows {
		// **DB に渡す前に弾く。** 制約違反を1つでも起こすと、トランザクション内では
		// 以降のコマンドが全部拒否される（SQLSTATE 25P02）
		if msg := v.Validate(); msg != "" {
			out.Errors = append(out.Errors, csvio.RowError{Line: i + 2, Message: msg})
			continue
		}

		tag, err := r.db.Exec(ctx, q, v.Date, v.WeightKg, v.BodyfatPct, v.Kcal, v.ProteinG, v.FatG, v.CarbG,
			v.SleepH, v.Steps, v.Fatigue, v.HrvMs, v.RestingHr, v.DeepSleepMin, v.Note)
		if err != nil {
			out.Errors = append(out.Errors, csvio.RowError{Line: i + 2, Message: err.Error()})
			continue
		}
		if tag.RowsAffected() == 0 {
			out.Skipped++
			continue
		}
		out.Imported++
	}

	return out, nil
}

// --- measures ---

// ExportMeasures は期間内の周囲長を返す。
func (r *Transfer) ExportMeasures(ctx context.Context, from, to *openapi_types.Date) ([]csvio.MeasureRow, error) {
	const q = `
		select date, neck_cm, shoulder_cm, chest_cm, waist_navel_cm,
		       hip_cm, arm_r_cm, thigh_r_cm, calf_r_cm
		from body_measurements
		where deleted_at is null
		  and ($1::date is null or date >= $1::date)
		  and ($2::date is null or date <= $2::date)
		order by date`

	rows, err := r.db.Query(ctx, q, dateOrNil(from), dateOrNil(to))
	if err != nil {
		return nil, fmt.Errorf("周囲長を引けない: %w", err)
	}
	defer rows.Close()

	out := make([]csvio.MeasureRow, 0, 64)
	for rows.Next() {
		var v csvio.MeasureRow
		err := rows.Scan(&v.Date, &v.NeckCm, &v.ShoulderCm, &v.ChestCm, &v.WaistNavelCm,
			&v.HipCm, &v.ArmRCm, &v.ThighRCm, &v.CalfRCm)
		if err != nil {
			return nil, fmt.Errorf("周囲長を読めない: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("周囲長を読めない: %w", err)
	}

	return out, nil
}

// ImportMeasures は周囲長を取り込む。
func (r *Transfer) ImportMeasures(ctx context.Context, rows []csvio.MeasureRow, on OnDuplicate) (ImportResult, error) {
	const qSkip = `
		insert into body_measurements
			(date, neck_cm, shoulder_cm, chest_cm, waist_navel_cm, hip_cm, arm_r_cm, thigh_r_cm, calf_r_cm)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		on conflict (date) where deleted_at is null do nothing`

	const qOverwrite = `
		insert into body_measurements
			(date, neck_cm, shoulder_cm, chest_cm, waist_navel_cm, hip_cm, arm_r_cm, thigh_r_cm, calf_r_cm)
		values ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		on conflict (date) where deleted_at is null do update set
			neck_cm = excluded.neck_cm, shoulder_cm = excluded.shoulder_cm,
			chest_cm = excluded.chest_cm, waist_navel_cm = excluded.waist_navel_cm,
			hip_cm = excluded.hip_cm, arm_r_cm = excluded.arm_r_cm,
			thigh_r_cm = excluded.thigh_r_cm, calf_r_cm = excluded.calf_r_cm,
			updated_at = now()`

	q := qSkip
	if on == OnDuplicateOverwrite {
		q = qOverwrite
	}

	out := ImportResult{Errors: []csvio.RowError{}}
	for i, v := range rows {
		if msg := v.Validate(); msg != "" {
			out.Errors = append(out.Errors, csvio.RowError{Line: i + 2, Message: msg})
			continue
		}

		tag, err := r.db.Exec(ctx, q, v.Date, v.NeckCm, v.ShoulderCm, v.ChestCm, v.WaistNavelCm,
			v.HipCm, v.ArmRCm, v.ThighRCm, v.CalfRCm)
		if err != nil {
			out.Errors = append(out.Errors, csvio.RowError{Line: i + 2, Message: err.Error()})
			continue
		}
		if tag.RowsAffected() == 0 {
			out.Skipped++
			continue
		}
		out.Imported++
	}

	return out, nil
}

// --- workouts ---

// ExportWorkouts は期間内のセットを種目名つきで返す。
func (r *Transfer) ExportWorkouts(ctx context.Context, from, to *openapi_types.Date) ([]csvio.WorkoutRow, error) {
	const q = `
		select ws.date, e.name, s.set_no, s.weight_kg, s.reps, s.rir
		from workout_sets s
		join workout_sessions ws on ws.id = s.session_id and ws.deleted_at is null
		join exercises e on e.id = s.exercise_id
		where s.deleted_at is null
		  and ($1::date is null or ws.date >= $1::date)
		  and ($2::date is null or ws.date <= $2::date)
		order by ws.date, s.set_no`

	rows, err := r.db.Query(ctx, q, dateOrNil(from), dateOrNil(to))
	if err != nil {
		return nil, fmt.Errorf("トレーニング記録を引けない: %w", err)
	}
	defer rows.Close()

	out := make([]csvio.WorkoutRow, 0, 1024)
	for rows.Next() {
		var v csvio.WorkoutRow
		if err := rows.Scan(&v.Date, &v.Exercise, &v.SetNo, &v.WeightKg, &v.Reps, &v.RIR); err != nil {
			return nil, fmt.Errorf("トレーニング記録を読めない: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("トレーニング記録を読めない: %w", err)
	}

	return out, nil
}

// ImportWorkouts はトレーニング記録を取り込む。
//
// CSV は種目を**名前**で持っているので、マスタに引き当てる。
// 見つからない種目は行ごとエラーにする。**勝手に作らない** ——
// 表記ゆれで似た種目が増えると時系列が分断される（openapi.yaml の Exercise.name）。
func (r *Transfer) ImportWorkouts(ctx context.Context, rows []csvio.WorkoutRow, on OnDuplicate) (ImportResult, error) {
	byName, err := r.exerciseIDsByName(ctx)
	if err != nil {
		return ImportResult{}, err
	}

	out := ImportResult{Errors: []csvio.RowError{}}
	sessions := map[time.Time]string{}

	for i, v := range rows {
		line := i + 2

		if msg := v.Validate(); msg != "" {
			out.Errors = append(out.Errors, csvio.RowError{Line: line, Message: msg})
			continue
		}

		exerciseID, ok := byName[v.Exercise]
		if !ok {
			out.Errors = append(out.Errors, csvio.RowError{
				Line:    line,
				Message: fmt.Sprintf("種目 %q がマスタに無い。先に登録するか表記を合わせる", v.Exercise),
			})
			continue
		}

		sessionID, ok := sessions[v.Date]
		if !ok {
			sessionID, err = r.ensureSession(ctx, v.Date)
			if err != nil {
				out.Errors = append(out.Errors, csvio.RowError{Line: line, Message: err.Error()})
				continue
			}
			sessions[v.Date] = sessionID
		}

		inserted, err := r.insertImportedSet(ctx, sessionID, exerciseID, v, on)
		switch {
		case err != nil:
			out.Errors = append(out.Errors, csvio.RowError{Line: line, Message: err.Error()})
		case inserted:
			out.Imported++
		default:
			out.Skipped++
		}
	}

	return out, nil
}

// exerciseIDsByName は種目名から id を引ける表を作る。
// 正規名だけでなく別表記（exercise_aliases）も含める。
// 過去に手で書いた CSV には別表記が混ざるため（要件 I-01）。
func (r *Transfer) exerciseIDsByName(ctx context.Context) (map[string]string, error) {
	const q = `
		select name, id::text from exercises where deleted_at is null
		union all
		select a.alias, a.exercise_id::text
		from exercise_aliases a
		join exercises e on e.id = a.exercise_id and e.deleted_at is null`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("種目マスタを引けない: %w", err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var name, id string
		if err := rows.Scan(&name, &id); err != nil {
			return nil, fmt.Errorf("種目マスタを読めない: %w", err)
		}
		out[name] = id
	}

	return out, rows.Err()
}

// ensureSession はその日のセッションを作るか、既にあれば返す。
func (r *Transfer) ensureSession(ctx context.Context, date time.Time) (string, error) {
	var id string
	err := r.db.QueryRow(ctx,
		`select id::text from workout_sessions where date = $1 and deleted_at is null order by created_at limit 1`,
		date).Scan(&id)
	if err == nil {
		return id, nil
	}

	err = r.db.QueryRow(ctx,
		`insert into workout_sessions (date, note) values ($1, $2) returning id::text`,
		date, "CSV 取り込み").Scan(&id)
	if err != nil {
		return "", fmt.Errorf("セッションを作れない: %w", err)
	}

	return id, nil
}

func (r *Transfer) insertImportedSet(
	ctx context.Context, sessionID, exerciseID string, v csvio.WorkoutRow, on OnDuplicate,
) (bool, error) {
	const qSkip = `
		insert into workout_sets (session_id, exercise_id, set_no, weight_kg, reps, rir)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (session_id, exercise_id, set_no) where deleted_at is null do nothing`

	const qOverwrite = `
		insert into workout_sets (session_id, exercise_id, set_no, weight_kg, reps, rir)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (session_id, exercise_id, set_no) where deleted_at is null do update set
			exercise_id = excluded.exercise_id, weight_kg = excluded.weight_kg,
			reps = excluded.reps, rir = excluded.rir, updated_at = $7`

	q := qSkip
	args := []any{sessionID, exerciseID, v.SetNo, v.WeightKg, v.Reps, v.RIR}
	if on == OnDuplicateOverwrite {
		q = qOverwrite
		args = append(args, timeutil.Now())
	}

	tag, err := r.db.Exec(ctx, q, args...)
	if err != nil {
		return false, err
	}

	return tag.RowsAffected() > 0, nil
}
