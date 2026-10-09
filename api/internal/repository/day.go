package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

// DayRecords は1日ぶんの元データ。判定（達成・差）は analytics が行う。
type DayRecords struct {
	Kcal     float64
	Consumed analytics.PFC
	// Targets は date 以前に始まった目標の履歴（StartsOn の昇順）
	Targets []analytics.StreakTarget
	// Workout はセットが1件でもある日だけ。無ければ nil
	Workout *DayWorkout
	// Body は体重・体脂肪率のどちらかがある日だけ。無ければ nil
	Body *DayBody
}

// DayWorkout はその日の筋トレ。
type DayWorkout struct {
	TemplateName *string
	// DayOrder は有効なルーティンの何日目か。ルーティンに無ければ nil
	DayOrder  *int
	Exercises []DayExercise
}

// DayExercise は種目ごとの集計。
type DayExercise struct {
	Name        string
	SetCount    int
	TopWeightKg float64
}

// DayBody はその日の体重・体脂肪率。
type DayBody struct {
	WeightKg   *float64
	BodyFatPct *float64
}

// Day は1日ぶんの要約の元データを引く。
type Day struct {
	db DBTX
}

// NewDay は Day を作る。
func NewDay(db DBTX) *Day {
	return &Day{db: db}
}

// Records は date の元データを返す。
//
// **4クエリで引く**（食事・目標・筋トレ・体組成）。カレンダーの日をタップするたびに
// 呼ばれるので、種目や食事ごとには引かない。
func (r *Day) Records(ctx context.Context, date time.Time) (DayRecords, error) {
	// **暦日の文字列で渡す。** time.Time は接続のタイムゾーンで日付が動きうる
	d := date.Format(time.DateOnly)

	var out DayRecords

	// 未入力（null）は 0 として足す。**1行必ず返る**ので、記録の無い日は 0 になる
	err := r.db.QueryRow(ctx, `
		select coalesce(sum(kcal), 0::numeric)::float8,
			coalesce(sum(protein_g), 0::numeric)::float8,
			coalesce(sum(fat_g), 0::numeric)::float8,
			coalesce(sum(carb_g), 0::numeric)::float8
		from meals
		where deleted_at is null and date = $1::date`, d,
	).Scan(&out.Kcal, &out.Consumed.ProteinG, &out.Consumed.FatG, &out.Consumed.CarbG)
	if err != nil {
		return DayRecords{}, fmt.Errorf("食事の合計を引けない: %w", err)
	}

	// 履歴は数件。date の日に効いている行（それ以前に始まったもの）が要る
	targets, err := r.db.Query(ctx, `
		select starts_on::text, protein_g::float8, fat_g::float8, carb_g::float8
		from manual_targets
		where starts_on <= $1::date
		order by starts_on`, d)
	if err != nil {
		return DayRecords{}, fmt.Errorf("目標の履歴を引けない: %w", err)
	}
	defer targets.Close()

	for targets.Next() {
		var t analytics.StreakTarget
		if err := targets.Scan(&t.StartsOn, &t.ProteinG, &t.FatG, &t.CarbG); err != nil {
			return DayRecords{}, fmt.Errorf("目標の履歴を読めない: %w", err)
		}
		out.Targets = append(out.Targets, t)
	}
	if err := targets.Err(); err != nil {
		return DayRecords{}, fmt.Errorf("目標の履歴を読めない: %w", err)
	}
	targets.Close()

	if out.Workout, err = r.workout(ctx, d); err != nil {
		return DayRecords{}, err
	}
	if out.Body, err = r.body(ctx, d); err != nil {
		return DayRecords{}, err
	}

	return out, nil
}

// workout は date の筋トレを返す。セットが1件も無ければ nil。
func (r *Day) workout(ctx context.Context, date string) (*DayWorkout, error) {
	// 何日目かは有効なルーティンから引く（有効なのは1つ。routines_one_active）。
	// ルーティンに無いテンプレートは left join で null になる
	rows, err := r.db.Query(ctx, `
		select t.name, rd.day_order, s.exercise_id, e.name, count(*), max(s.weight_kg)::float8
		from workout_sessions ws
			join workout_sets s on s.session_id = ws.id and s.deleted_at is null
			join exercises e on e.id = s.exercise_id
			left join templates t on t.id = ws.template_id
			left join routine_days rd on rd.template_id = ws.template_id
				and rd.routine_id = (select id from routines where is_active and deleted_at is null)
		where ws.deleted_at is null and ws.date = $1::date
		group by ws.id, t.name, rd.day_order, s.exercise_id, e.name
		order by ws.created_at, min(s.set_no), e.name`, date)
	if err != nil {
		return nil, fmt.Errorf("筋トレを引けない: %w", err)
	}
	defer rows.Close()

	var out *DayWorkout
	byExercise := map[uuid.UUID]int{} // 同じ日に複数セッションがあっても種目は1行にまとめる
	for rows.Next() {
		var (
			tname    *string
			dayOrder *int
			id       uuid.UUID
			ex       DayExercise
		)
		if err := rows.Scan(&tname, &dayOrder, &id, &ex.Name, &ex.SetCount, &ex.TopWeightKg); err != nil {
			return nil, fmt.Errorf("筋トレを読めない: %w", err)
		}

		if out == nil {
			out = &DayWorkout{TemplateName: tname, DayOrder: dayOrder}
		}
		if i, ok := byExercise[id]; ok {
			out.Exercises[i].SetCount += ex.SetCount
			out.Exercises[i].TopWeightKg = max(out.Exercises[i].TopWeightKg, ex.TopWeightKg)

			continue
		}
		byExercise[id] = len(out.Exercises)
		out.Exercises = append(out.Exercises, ex)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("筋トレを読めない: %w", err)
	}

	return out, nil
}

// body は date の体重・体脂肪率を返す。どちらも無ければ nil。
func (r *Day) body(ctx context.Context, date string) (*DayBody, error) {
	var b DayBody
	err := r.db.QueryRow(ctx, `
		select weight_kg::float8, bodyfat_pct::float8
		from daily_metrics
		where deleted_at is null and date = $1::date`, date).Scan(&b.WeightKg, &b.BodyFatPct)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // 記録が無い日は nil で表す
	}
	if err != nil {
		return nil, fmt.Errorf("体組成を引けない: %w", err)
	}
	if b.WeightKg == nil && b.BodyFatPct == nil {
		return nil, nil //nolint:nilnil // 他の列だけの行は体組成なし
	}

	return &b, nil
}
