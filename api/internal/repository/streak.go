package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

// StreakInputs は日別の達成判定に使う元データ。
type StreakInputs struct {
	// Targets は StartsOn の昇順
	Targets []analytics.StreakTarget
	// Consumed は日付（YYYY-MM-DD）ごとの P・F・C の合計
	Consumed map[string]analytics.PFC
	// Trained は1セットでも記録した日
	Trained map[string]bool
}

// Streak は日別の達成フラグの元データを引く。
type Streak struct {
	db DBTX
}

// NewStreak は Streak を作る。
func NewStreak(db DBTX) *Streak {
	return &Streak{db: db}
}

// Inputs は from〜to の元データを返す。
//
// **期間を3クエリで引く**（食事・筋トレ・目標）。日ごとに引くと366日で千往復を超える。
// 目標は `starts_on <= to` を全部返す。履歴は数件で、from の日に効いている行
// （from より前に始まったもの）も要るので、from では絞らない。
func (r *Streak) Inputs(ctx context.Context, from, to time.Time) (StreakInputs, error) {
	// **暦日の文字列で渡す。** time.Time は接続のタイムゾーンで日付が動きうる
	f, t := from.Format(time.DateOnly), to.Format(time.DateOnly)

	out := StreakInputs{
		Consumed: map[string]analytics.PFC{},
		Trained:  map[string]bool{},
	}

	// 未入力（null）は 0 として足す。0 を入れて辻褄を合わせるより、合計が少なく出る方がよい
	meals, err := r.db.Query(ctx, `
		select date::text,
			coalesce(sum(protein_g), 0::numeric)::float8,
			coalesce(sum(fat_g), 0::numeric)::float8,
			coalesce(sum(carb_g), 0::numeric)::float8
		from meals
		where deleted_at is null and date between $1::date and $2::date
		group by date`, f, t)
	if err != nil {
		return StreakInputs{}, fmt.Errorf("食事の合計を引けない: %w", err)
	}
	defer meals.Close()

	for meals.Next() {
		var date string
		var v analytics.PFC
		if err := meals.Scan(&date, &v.ProteinG, &v.FatG, &v.CarbG); err != nil {
			return StreakInputs{}, fmt.Errorf("食事の合計を読めない: %w", err)
		}
		out.Consumed[date] = v
	}
	if err := meals.Err(); err != nil {
		return StreakInputs{}, fmt.Errorf("食事の合計を読めない: %w", err)
	}
	meals.Close()

	trained, err := r.db.Query(ctx, `
		select distinct ws.date::text
		from workout_sets s
			join workout_sessions ws on ws.id = s.session_id and ws.deleted_at is null
		where s.deleted_at is null and ws.date between $1::date and $2::date`, f, t)
	if err != nil {
		return StreakInputs{}, fmt.Errorf("筋トレの日を引けない: %w", err)
	}
	defer trained.Close()

	for trained.Next() {
		var date string
		if err := trained.Scan(&date); err != nil {
			return StreakInputs{}, fmt.Errorf("筋トレの日を読めない: %w", err)
		}
		out.Trained[date] = true
	}
	if err := trained.Err(); err != nil {
		return StreakInputs{}, fmt.Errorf("筋トレの日を読めない: %w", err)
	}
	trained.Close()

	targets, err := r.db.Query(ctx, `
		select starts_on::text, protein_g::float8, fat_g::float8, carb_g::float8
		from manual_targets
		where starts_on <= $1::date
		order by starts_on`, t)
	if err != nil {
		return StreakInputs{}, fmt.Errorf("目標の履歴を引けない: %w", err)
	}
	defer targets.Close()

	for targets.Next() {
		var v analytics.StreakTarget
		if err := targets.Scan(&v.StartsOn, &v.ProteinG, &v.FatG, &v.CarbG); err != nil {
			return StreakInputs{}, fmt.Errorf("目標の履歴を読めない: %w", err)
		}
		out.Targets = append(out.Targets, v)
	}
	if err := targets.Err(); err != nil {
		return StreakInputs{}, fmt.Errorf("目標の履歴を読めない: %w", err)
	}

	return out, nil
}
