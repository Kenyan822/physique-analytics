package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// Contest は大会へのアクセス（要件 P-04）。
//
// カウントダウンと必要ペース判定（要件 A-10）の入力になる。
type Contest struct {
	db DBTX
}

// NewContest は Contest を作る。
func NewContest(db DBTX) *Contest {
	return &Contest{db: db}
}

const contestColumns = `id, held_on, category, target_bf_pct, goal,
	created_at, updated_at, deleted_at`

// List は大会を開催日の昇順で返す。
func (r *Contest) List(ctx context.Context) ([]openapi.Contest, error) {
	const q = `select ` + contestColumns + ` from contests
		where deleted_at is null order by held_on`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("大会の一覧を引けない: %w", err)
	}
	defer rows.Close()

	out := make([]openapi.Contest, 0, 8)
	for rows.Next() {
		c, err := scanContest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("大会の一覧を読めない: %w", err)
	}

	return out, nil
}

// Next は基準日以降で一番近い大会を返す。
//
// **当日も含める。** 当日にカウントダウンが消えるのは不自然。
func (r *Contest) Next(ctx context.Context, asof openapi_types.Date) (openapi.Contest, error) {
	const q = `select ` + contestColumns + ` from contests
		where deleted_at is null and held_on >= $1
		order by held_on limit 1`

	c, err := scanContest(r.db.QueryRow(ctx, q, asof.Time))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.Contest{}, fmt.Errorf("次の大会: %w", ErrNotFound)
	}
	if err != nil {
		return openapi.Contest{}, fmt.Errorf("次の大会を取得できない: %w", err)
	}

	return c, nil
}

// Create は大会を登録する。
func (r *Contest) Create(ctx context.Context, in openapi.ContestInput) (openapi.Contest, error) {
	const q = `insert into contests (held_on, category, target_bf_pct, goal)
		values ($1, $2, $3, $4) returning ` + contestColumns

	c, err := scanContest(r.db.QueryRow(ctx, q, in.HeldOn.Time, in.Category, in.TargetBfPct, in.Goal))
	if err != nil {
		return openapi.Contest{}, fmt.Errorf("大会を登録できない: %w", err)
	}

	return c, nil
}

// Update は大会を更新する。
func (r *Contest) Update(ctx context.Context, id uuid.UUID, in openapi.ContestInput) (openapi.Contest, error) {
	const q = `update contests set held_on = $2, category = $3, target_bf_pct = $4,
			goal = $5, updated_at = $6
		where id = $1 and deleted_at is null
		returning ` + contestColumns

	c, err := scanContest(r.db.QueryRow(ctx, q, id, in.HeldOn.Time, in.Category,
		in.TargetBfPct, in.Goal, timeutil.Now()))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.Contest{}, fmt.Errorf("大会 %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return openapi.Contest{}, fmt.Errorf("大会を更新できない: %w", err)
	}

	return c, nil
}

// SoftDelete は大会を論理削除する（ADR-0014）。
func (r *Contest) SoftDelete(ctx context.Context, id uuid.UUID) error {
	const q = `update contests set deleted_at = $2, updated_at = $2
		where id = $1 and deleted_at is null`

	tag, err := r.db.Exec(ctx, q, id, timeutil.Now())
	if err != nil {
		return fmt.Errorf("大会を削除できない: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("大会 %s: %w", id, ErrNotFound)
	}

	return nil
}

func scanContest(row pgx.Row) (openapi.Contest, error) {
	var c openapi.Contest
	err := row.Scan(&c.Id, &c.HeldOn.Time, &c.Category, &c.TargetBfPct, &c.Goal,
		&c.CreatedAt, &c.UpdatedAt, &c.DeletedAt)
	if err != nil {
		return openapi.Contest{}, err
	}

	return c, nil
}
