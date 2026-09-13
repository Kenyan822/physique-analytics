package repository

import (
	"context"
	"fmt"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// ListBlocks は計画のブロックを順番に返す（要件 P-02）。
func (r *Plan) ListBlocks(ctx context.Context) ([]openapi.PlanBlock, error) {
	const q = `select name, months, lbm_delta_kg_per_month, bodyfat_pct_end, focus
		from plan_blocks order by block_order`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("計画のブロックを引けない: %w", err)
	}
	defer rows.Close()

	out := make([]openapi.PlanBlock, 0, 16)
	for rows.Next() {
		var b openapi.PlanBlock
		if err := rows.Scan(&b.Name, &b.Months, &b.LbmDeltaKgPerMonth, &b.BodyfatPctEnd, &b.Focus); err != nil {
			return nil, fmt.Errorf("計画のブロックを読めない: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("計画のブロックを読めない: %w", err)
	}

	return out, nil
}

// PutBlocks はブロックをまるごと置き換える。順序は配列の並びで決まる。
func (r *Plan) PutBlocks(ctx context.Context, blocks []openapi.PlanBlock) ([]openapi.PlanBlock, error) {
	if _, err := r.db.Exec(ctx, `delete from plan_blocks`); err != nil {
		return nil, fmt.Errorf("計画のブロックを消せない: %w", err)
	}

	const q = `insert into plan_blocks
		(block_order, name, months, lbm_delta_kg_per_month, bodyfat_pct_end, focus, updated_at)
		values ($1, $2, $3, $4, $5, $6, $7)`
	now := timeutil.Now()
	for i, b := range blocks {
		if _, err := r.db.Exec(ctx, q, i+1, b.Name, b.Months,
			b.LbmDeltaKgPerMonth, b.BodyfatPctEnd, b.Focus, now); err != nil {
			return nil, fmt.Errorf("ブロック %q を保存できない: %w", b.Name, err)
		}
	}

	return r.ListBlocks(ctx)
}

// ToAnalyticsBlocks は analytics が受け取る形に変換する。
func ToAnalyticsBlocks(blocks []openapi.PlanBlock) []analytics.PlanBlock {
	out := make([]analytics.PlanBlock, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, analytics.PlanBlock{
			Name:               b.Name,
			Months:             b.Months,
			LbmDeltaKgPerMonth: float64(b.LbmDeltaKgPerMonth),
			BodyfatPctEnd:      float64(b.BodyfatPctEnd),
		})
	}

	return out
}
