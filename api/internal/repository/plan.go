package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// Plan は計画の設定へのアクセス（要件 P-01 / P-05）。
//
// これまで private/config.json にあり、手元の MCP からしか読めなかった。
// 栄養目標の表示（要件 N-05）と設定画面のために DB に移した。
type Plan struct {
	db DBTX
}

// NewPlan は Plan を作る。
func NewPlan(db DBTX) *Plan {
	return &Plan{db: db}
}

// Get は計画の設定を返す。
//
// 未設定でも空を返さない。栄養パラメータと MEV/MRV はマイグレーションで
// 既定値を入れてあり、空だと全部位が「判定不能」になって分析が成立しない。
func (r *Plan) Get(ctx context.Context) (openapi.Plan, error) {
	out := openapi.Plan{
		Phases:       []openapi.PlanPhase{},
		VolumeRanges: []openapi.VolumeRange{},
	}

	if err := r.scanProfile(ctx, &out); err != nil {
		return openapi.Plan{}, err
	}
	if err := r.scanNutrition(ctx, &out); err != nil {
		return openapi.Plan{}, err
	}
	phases, err := r.listPhases(ctx)
	if err != nil {
		return openapi.Plan{}, err
	}
	out.Phases = phases

	ranges, err := r.listVolumeRanges(ctx)
	if err != nil {
		return openapi.Plan{}, err
	}
	out.VolumeRanges = ranges

	return out, nil
}

// Put は計画の設定をまるごと置き換える。
//
// **部分更新にしない。** フェーズや MEV/MRV を差分で更新すると、
// 「消したつもりが残っている」が起きる。設定は頻繁には変えない。
func (r *Plan) Put(ctx context.Context, in openapi.PlanInput) (openapi.Plan, error) {
	now := timeutil.Now()

	const profileQ = `
		insert into profile (id, height_cm, start_date,
			baseline_weight_kg, baseline_bodyfat_pct, baseline_month, updated_at)
		values (true, $1, $2, $3, $4, $5, $6)
		on conflict (id) do update set
			height_cm = excluded.height_cm,
			start_date = excluded.start_date,
			baseline_weight_kg = excluded.baseline_weight_kg,
			baseline_bodyfat_pct = excluded.baseline_bodyfat_pct,
			baseline_month = excluded.baseline_month,
			updated_at = excluded.updated_at`
	if _, err := r.db.Exec(ctx, profileQ, in.HeightCm, dateOrNil(in.StartDate),
		in.BaselineWeightKg, in.BaselineBodyfatPct, monthToDate(in.BaselineMonth), now); err != nil {
		return openapi.Plan{}, fmt.Errorf("身体の基本値を保存できない: %w", err)
	}

	if err := r.putNutrition(ctx, in.Nutrition, now); err != nil {
		return openapi.Plan{}, err
	}
	if err := r.replacePhases(ctx, in.Phases); err != nil {
		return openapi.Plan{}, err
	}
	if err := r.replaceVolumeRanges(ctx, in.VolumeRanges, now); err != nil {
		return openapi.Plan{}, err
	}

	return r.Get(ctx)
}

// GoalAt は日付が属するフェーズの目標ペースを返す。
//
// **該当が無ければ ok=false。** 0 を返すと維持期と区別が付かず、
// 停滞判定（維持期は横ばいが正常）が変わってしまう。
func GoalAt(phases []openapi.PlanPhase, date time.Time) (goalKgPerWeek float64, ok bool) {
	day := date.Format(time.DateOnly)

	for _, p := range phases {
		if p.StartsOn.Format(time.DateOnly) <= day && day <= p.EndsOn.Format(time.DateOnly) {
			return float64(p.GoalKgPerWeek), true
		}
	}

	return 0, false
}

func (r *Plan) scanProfile(ctx context.Context, out *openapi.Plan) error {
	const q = `select height_cm, start_date,
		baseline_weight_kg, baseline_bodyfat_pct, baseline_month
		from profile where id = true`

	var start, baselineMonth *time.Time
	err := r.db.QueryRow(ctx, q).Scan(&out.HeightCm, &start,
		&out.BaselineWeightKg, &out.BaselineBodyfatPct, &baselineMonth)
	if errors.Is(err, pgx.ErrNoRows) {
		// 未設定。身長が無いと FFMI は出せないが、他の分析は動く
		return nil
	}
	if err != nil {
		return fmt.Errorf("身体の基本値を読めない: %w", err)
	}
	if start != nil {
		out.StartDate = &openapi_types.Date{Time: *start}
	}
	if baselineMonth != nil {
		m := baselineMonth.Format("2006-01")
		out.BaselineMonth = &m
	}

	return nil
}

// monthToDate は YYYY-MM をその月の1日にする。
// DB は date 型で持つ（月だけの型が無く、文字列だと範囲検索が効かない）。
func monthToDate(month *string) *time.Time {
	if month == nil || *month == "" {
		return nil
	}

	t, err := time.Parse("2006-01", *month)
	if err != nil {
		return nil
	}

	return &t
}

func (r *Plan) scanNutrition(ctx context.Context, out *openapi.Plan) error {
	const q = `
		select cut_protein_g_per_kg, cut_fat_g_per_kg,
		       deep_cut_protein_g_per_kg, deep_cut_fat_g_per_kg,
		       bulk_protein_g_per_kg, bulk_fat_g_per_kg,
		       deep_cut_bf_threshold, carb_min_g
		from nutrition_settings where id = true`

	n := &out.Nutrition
	err := r.db.QueryRow(ctx, q).Scan(
		&n.Cut.ProteinGPerKg, &n.Cut.FatGPerKg,
		&n.DeepCut.ProteinGPerKg, &n.DeepCut.FatGPerKg,
		&n.Bulk.ProteinGPerKg, &n.Bulk.FatGPerKg,
		&n.DeepCutBfThreshold, &n.CarbMinG)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("栄養パラメータを読めない: %w", err)
	}

	return nil
}

func (r *Plan) putNutrition(ctx context.Context, n openapi.NutritionSettings, now time.Time) error {
	const q = `
		insert into nutrition_settings
			(id, cut_protein_g_per_kg, cut_fat_g_per_kg,
			 deep_cut_protein_g_per_kg, deep_cut_fat_g_per_kg,
			 bulk_protein_g_per_kg, bulk_fat_g_per_kg,
			 deep_cut_bf_threshold, carb_min_g, updated_at)
		values (true, $1, $2, $3, $4, $5, $6, $7, $8, $9)
		on conflict (id) do update set
			cut_protein_g_per_kg = excluded.cut_protein_g_per_kg,
			cut_fat_g_per_kg = excluded.cut_fat_g_per_kg,
			deep_cut_protein_g_per_kg = excluded.deep_cut_protein_g_per_kg,
			deep_cut_fat_g_per_kg = excluded.deep_cut_fat_g_per_kg,
			bulk_protein_g_per_kg = excluded.bulk_protein_g_per_kg,
			bulk_fat_g_per_kg = excluded.bulk_fat_g_per_kg,
			deep_cut_bf_threshold = excluded.deep_cut_bf_threshold,
			carb_min_g = excluded.carb_min_g,
			updated_at = excluded.updated_at`

	_, err := r.db.Exec(ctx, q,
		n.Cut.ProteinGPerKg, n.Cut.FatGPerKg,
		n.DeepCut.ProteinGPerKg, n.DeepCut.FatGPerKg,
		n.Bulk.ProteinGPerKg, n.Bulk.FatGPerKg,
		n.DeepCutBfThreshold, n.CarbMinG, now)
	if err != nil {
		return fmt.Errorf("栄養パラメータを保存できない: %w", err)
	}

	return nil
}

func (r *Plan) listPhases(ctx context.Context) ([]openapi.PlanPhase, error) {
	// フェーズは時系列で読むものなので期間の昇順で返す
	const q = `select id, name, starts_on, ends_on, goal_kg_per_week
		from plan_phases order by starts_on`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("フェーズを引けない: %w", err)
	}
	defer rows.Close()

	out := make([]openapi.PlanPhase, 0, 16)
	for rows.Next() {
		var p openapi.PlanPhase
		if err := rows.Scan(&p.Id, &p.Name, &p.StartsOn.Time, &p.EndsOn.Time, &p.GoalKgPerWeek); err != nil {
			return nil, fmt.Errorf("フェーズを読めない: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("フェーズを読めない: %w", err)
	}

	return out, nil
}

func (r *Plan) replacePhases(ctx context.Context, phases []openapi.PlanPhase) error {
	// 計画そのものは記録ではないので論理削除しない。過去の版を残す意味が無い
	if _, err := r.db.Exec(ctx, `delete from plan_phases`); err != nil {
		return fmt.Errorf("フェーズを消せない: %w", err)
	}

	const q = `insert into plan_phases (name, starts_on, ends_on, goal_kg_per_week)
		values ($1, $2, $3, $4)`
	for _, p := range phases {
		if _, err := r.db.Exec(ctx, q, p.Name, p.StartsOn.Time, p.EndsOn.Time, p.GoalKgPerWeek); err != nil {
			return fmt.Errorf("フェーズ %q を保存できない: %w", p.Name, err)
		}
	}

	return nil
}

func (r *Plan) listVolumeRanges(ctx context.Context) ([]openapi.VolumeRange, error) {
	const q = `select muscle_group, mev, mrv from volume_ranges order by muscle_group`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("MEV/MRV を引けない: %w", err)
	}
	defer rows.Close()

	out := make([]openapi.VolumeRange, 0, 16)
	for rows.Next() {
		var v openapi.VolumeRange
		var mg string
		if err := rows.Scan(&mg, &v.Mev, &v.Mrv); err != nil {
			return nil, fmt.Errorf("MEV/MRV を読めない: %w", err)
		}
		v.MuscleGroup = openapi.MuscleGroup(mg)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("MEV/MRV を読めない: %w", err)
	}

	return out, nil
}

func (r *Plan) replaceVolumeRanges(ctx context.Context, ranges []openapi.VolumeRange, now time.Time) error {
	const q = `
		insert into volume_ranges (muscle_group, mev, mrv, updated_at)
		values ($1, $2, $3, $4)
		on conflict (muscle_group) do update set
			mev = excluded.mev, mrv = excluded.mrv, updated_at = excluded.updated_at`

	// **既定値を消さない。** 送られなかった部位は既定のまま残す。
	// 全消しにすると、1部位だけ直したつもりで他が判定不能になる
	for _, v := range ranges {
		if _, err := r.db.Exec(ctx, q, string(v.MuscleGroup), v.Mev, v.Mrv, now); err != nil {
			return fmt.Errorf("MEV/MRV（%s）を保存できない: %w", v.MuscleGroup, err)
		}
	}

	return nil
}
