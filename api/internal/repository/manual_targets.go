package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// ManualTargets は手で決めた摂取目標（要件 N-05）へのアクセス。
//
// **単一行。** フェーズ単位で変えるもので、日ごとに持つのは過剰。
type ManualTargets struct {
	db DBTX
}

// NewManualTargets は ManualTargets を作る。
func NewManualTargets(db DBTX) *ManualTargets {
	return &ManualTargets{db: db}
}

// Get は設定を返す。**設定していなければ nil**（エラーにしない）。
func (r *ManualTargets) Get(ctx context.Context) (*openapi.ManualTargets, error) {
	const q = `select protein_g, fat_g, carb_g, updated_at from manual_targets where id`

	t, err := scanManualTargets(r.db.QueryRow(ctx, q))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("目標を取得できない: %w", err)
	}

	return &t, nil
}

// Put は設定を保存する。既にあれば置き換える。
func (r *ManualTargets) Put(ctx context.Context, in openapi.ManualTargets) (openapi.ManualTargets, error) {
	const q = `
		insert into manual_targets (id, protein_g, fat_g, carb_g, updated_at)
		values (true, $1, $2, $3, $4)
		on conflict (id) do update set
			protein_g = excluded.protein_g,
			fat_g = excluded.fat_g,
			carb_g = excluded.carb_g,
			updated_at = excluded.updated_at
		returning protein_g, fat_g, carb_g, updated_at`

	t, err := scanManualTargets(
		r.db.QueryRow(ctx, q, in.ProteinG, in.FatG, in.CarbG, timeutil.Now()))
	if err != nil {
		return openapi.ManualTargets{}, fmt.Errorf("目標を保存できない: %w", err)
	}

	return t, nil
}

// Delete は設定を消す。**無くてもエラーにしない**（DELETE は冪等）。
func (r *ManualTargets) Delete(ctx context.Context) error {
	if _, err := r.db.Exec(ctx, `delete from manual_targets where id`); err != nil {
		return fmt.Errorf("目標を消せない: %w", err)
	}

	return nil
}

// scanManualTargets は1行読む。
//
// **kcal は列に持たない。** PFC から計算できる（Atwater 4/9/4）。
// 持つと手入力と計算値が食い違ったときにどちらが正か決められなくなる
func scanManualTargets(row pgx.Row) (openapi.ManualTargets, error) {
	var t openapi.ManualTargets
	if err := row.Scan(&t.ProteinG, &t.FatG, &t.CarbG, &t.UpdatedAt); err != nil {
		return openapi.ManualTargets{}, err
	}

	kcal := analytics.KcalFromMacros(float64(t.ProteinG), float64(t.FatG), float64(t.CarbG))
	t.Kcal = &kcal

	return t, nil
}
