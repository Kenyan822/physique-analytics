package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// Body は日次記録と周囲長へのアクセス。
//
// どちらも「日付が主キー」の1日1行で、更新は upsert になる。
// セッションと違って作成と更新を分ける意味がないので、Put にまとめている。
type Body struct {
	db DBTX
}

// NewBody は Body を作る。
func NewBody(db DBTX) *Body {
	return &Body{db: db}
}

// DailyInput は日次記録の入力。
//
// **nil は「変更しない」を意味する。** 朝に体重、夜に食事、と分けて
// 入力するため、1回の Put で全項目を送らせるわけにいかない。
// 値を消したいときは SoftDeleteDaily してから入れ直す。
type DailyInput struct {
	Date         openapi_types.Date
	WeightKg     *float32
	BodyfatPct   *float32
	Kcal         *int
	ProteinG     *int
	FatG         *int
	CarbG        *int
	SleepH       *float32
	Steps        *int
	Fatigue      *int
	HrvMs        *int
	RestingHr    *int
	DeepSleepMin *int
	Note         *string
}

// MeasurementInput は周囲長の入力。nil の扱いは DailyInput と同じ。
type MeasurementInput struct {
	Date         openapi_types.Date
	NeckCm       *float32
	ShoulderCm   *float32
	ChestCm      *float32
	WaistNavelCm *float32
	HipCm        *float32
	ArmRCm       *float32
	ThighRCm     *float32
	CalfRCm      *float32
}

const dailyColumns = `id, date, weight_kg, bodyfat_pct, kcal, protein_g, fat_g, carb_g,
	sleep_h, steps, fatigue, hrv_ms, resting_hr, deep_sleep_min, note,
	created_at, updated_at, deleted_at`

const measurementColumns = `id, date, neck_cm, shoulder_cm, chest_cm, waist_navel_cm,
	hip_cm, arm_r_cm, thigh_r_cm, calf_r_cm, created_at, updated_at, deleted_at`

// ListDaily は期間内の日次記録を新しい順に返す。from / to は nil で無制限。
func (r *Body) ListDaily(ctx context.Context, from, to *openapi_types.Date) ([]openapi.DailyMetrics, error) {
	const q = `
		select ` + dailyColumns + `
		from daily_metrics
		where deleted_at is null
		  and ($1::date is null or date >= $1::date)
		  and ($2::date is null or date <= $2::date)
		order by date desc`

	rows, err := r.db.Query(ctx, q, dateOrNil(from), dateOrNil(to))
	if err != nil {
		return nil, fmt.Errorf("日次記録の一覧を引けない: %w", err)
	}
	defer rows.Close()

	out := make([]openapi.DailyMetrics, 0, 32)
	for rows.Next() {
		d, err := scanDaily(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("日次記録の一覧を読めない: %w", err)
	}

	return out, nil
}

// GetDaily は1日分の記録を返す。
func (r *Body) GetDaily(ctx context.Context, date openapi_types.Date) (openapi.DailyMetrics, error) {
	const q = `select ` + dailyColumns + ` from daily_metrics
		where date = $1 and deleted_at is null`

	d, err := scanDaily(r.db.QueryRow(ctx, q, date.Time))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.DailyMetrics{}, fmt.Errorf("日次記録 %s: %w", date.Format("2006-01-02"), ErrNotFound)
	}
	if err != nil {
		return openapi.DailyMetrics{}, fmt.Errorf("日次記録を取得できない: %w", err)
	}

	return d, nil
}

// PutDaily は日付をキーに upsert する。nil の項目は既存値を残す。
func (r *Body) PutDaily(ctx context.Context, in DailyInput) (openapi.DailyMetrics, error) {
	// **coalesce(excluded.x, daily_metrics.x) が要点。**
	// 単純な do update set x = excluded.x にすると、送らなかった項目が
	// null で潰れる。朝に入れた体重が夜の食事入力で消えることになる
	const q = `
		insert into daily_metrics
			(date, weight_kg, bodyfat_pct, kcal, protein_g, fat_g, carb_g,
			 sleep_h, steps, fatigue, hrv_ms, resting_hr, deep_sleep_min, note)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		on conflict (date) where deleted_at is null do update set
			weight_kg      = coalesce(excluded.weight_kg, daily_metrics.weight_kg),
			bodyfat_pct    = coalesce(excluded.bodyfat_pct, daily_metrics.bodyfat_pct),
			kcal           = coalesce(excluded.kcal, daily_metrics.kcal),
			protein_g      = coalesce(excluded.protein_g, daily_metrics.protein_g),
			fat_g          = coalesce(excluded.fat_g, daily_metrics.fat_g),
			carb_g         = coalesce(excluded.carb_g, daily_metrics.carb_g),
			sleep_h        = coalesce(excluded.sleep_h, daily_metrics.sleep_h),
			steps          = coalesce(excluded.steps, daily_metrics.steps),
			fatigue        = coalesce(excluded.fatigue, daily_metrics.fatigue),
			hrv_ms         = coalesce(excluded.hrv_ms, daily_metrics.hrv_ms),
			resting_hr     = coalesce(excluded.resting_hr, daily_metrics.resting_hr),
			deep_sleep_min = coalesce(excluded.deep_sleep_min, daily_metrics.deep_sleep_min),
			note           = coalesce(excluded.note, daily_metrics.note),
			updated_at     = $15
		returning ` + dailyColumns

	d, err := scanDaily(r.db.QueryRow(ctx, q,
		in.Date.Time, in.WeightKg, in.BodyfatPct, in.Kcal, in.ProteinG, in.FatG, in.CarbG,
		in.SleepH, in.Steps, in.Fatigue, in.HrvMs, in.RestingHr, in.DeepSleepMin, in.Note,
		timeutil.Now(),
	))
	if err != nil {
		return openapi.DailyMetrics{}, fmt.Errorf("日次記録を保存できない: %w", err)
	}

	return d, nil
}

// SoftDeleteDaily は1日分を論理削除する（ADR-0014）。
func (r *Body) SoftDeleteDaily(ctx context.Context, date openapi_types.Date) error {
	const q = `update daily_metrics set deleted_at = $2, updated_at = $2
		where date = $1 and deleted_at is null`

	tag, err := r.db.Exec(ctx, q, date.Time, timeutil.Now())
	if err != nil {
		return fmt.Errorf("日次記録を削除できない: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("日次記録 %s: %w", date.Format("2006-01-02"), ErrNotFound)
	}

	return nil
}

// ListMeasurements は期間内の周囲長を新しい順に返す。
func (r *Body) ListMeasurements(ctx context.Context, from, to *openapi_types.Date) ([]openapi.BodyMeasurement, error) {
	const q = `
		select ` + measurementColumns + `
		from body_measurements
		where deleted_at is null
		  and ($1::date is null or date >= $1::date)
		  and ($2::date is null or date <= $2::date)
		order by date desc`

	rows, err := r.db.Query(ctx, q, dateOrNil(from), dateOrNil(to))
	if err != nil {
		return nil, fmt.Errorf("周囲長の一覧を引けない: %w", err)
	}
	defer rows.Close()

	out := make([]openapi.BodyMeasurement, 0, 32)
	for rows.Next() {
		m, err := scanMeasurement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("周囲長の一覧を読めない: %w", err)
	}

	return out, nil
}

// PutMeasurement は日付をキーに upsert する。nil の項目は既存値を残す。
func (r *Body) PutMeasurement(ctx context.Context, in MeasurementInput) (openapi.BodyMeasurement, error) {
	const q = `
		insert into body_measurements
			(date, neck_cm, shoulder_cm, chest_cm, waist_navel_cm,
			 hip_cm, arm_r_cm, thigh_r_cm, calf_r_cm)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		on conflict (date) where deleted_at is null do update set
			neck_cm        = coalesce(excluded.neck_cm, body_measurements.neck_cm),
			shoulder_cm    = coalesce(excluded.shoulder_cm, body_measurements.shoulder_cm),
			chest_cm       = coalesce(excluded.chest_cm, body_measurements.chest_cm),
			waist_navel_cm = coalesce(excluded.waist_navel_cm, body_measurements.waist_navel_cm),
			hip_cm         = coalesce(excluded.hip_cm, body_measurements.hip_cm),
			arm_r_cm       = coalesce(excluded.arm_r_cm, body_measurements.arm_r_cm),
			thigh_r_cm     = coalesce(excluded.thigh_r_cm, body_measurements.thigh_r_cm),
			calf_r_cm      = coalesce(excluded.calf_r_cm, body_measurements.calf_r_cm),
			updated_at     = $10
		returning ` + measurementColumns

	m, err := scanMeasurement(r.db.QueryRow(ctx, q,
		in.Date.Time, in.NeckCm, in.ShoulderCm, in.ChestCm, in.WaistNavelCm,
		in.HipCm, in.ArmRCm, in.ThighRCm, in.CalfRCm, timeutil.Now(),
	))
	if err != nil {
		return openapi.BodyMeasurement{}, fmt.Errorf("周囲長を保存できない: %w", err)
	}

	return m, nil
}

// LatestMeasurement は日付が一番新しい周囲長を返す（要件 B-03）。
func (r *Body) LatestMeasurement(ctx context.Context) (openapi.BodyMeasurement, error) {
	const q = `select ` + measurementColumns + ` from body_measurements
		where deleted_at is null order by date desc limit 1`

	m, err := scanMeasurement(r.db.QueryRow(ctx, q))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.BodyMeasurement{}, fmt.Errorf("周囲長の記録: %w", ErrNotFound)
	}
	if err != nil {
		return openapi.BodyMeasurement{}, fmt.Errorf("周囲長を取得できない: %w", err)
	}

	return m, nil
}

func scanDaily(row pgx.Row) (openapi.DailyMetrics, error) {
	var d openapi.DailyMetrics
	err := row.Scan(
		&d.Id, &d.Date.Time, &d.WeightKg, &d.BodyfatPct, &d.Kcal, &d.ProteinG, &d.FatG, &d.CarbG,
		&d.SleepH, &d.Steps, &d.Fatigue, &d.HrvMs, &d.RestingHr, &d.DeepSleepMin, &d.Note,
		&d.CreatedAt, &d.UpdatedAt, &d.DeletedAt,
	)
	if err != nil {
		return openapi.DailyMetrics{}, err
	}

	return d, nil
}

func scanMeasurement(row pgx.Row) (openapi.BodyMeasurement, error) {
	var m openapi.BodyMeasurement
	err := row.Scan(
		&m.Id, &m.Date.Time, &m.NeckCm, &m.ShoulderCm, &m.ChestCm, &m.WaistNavelCm,
		&m.HipCm, &m.ArmRCm, &m.ThighRCm, &m.CalfRCm,
		&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt,
	)
	if err != nil {
		return openapi.BodyMeasurement{}, err
	}

	return m, nil
}
