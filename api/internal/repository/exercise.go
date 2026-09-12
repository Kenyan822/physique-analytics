package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// Exercise は種目マスタへのアクセス。
type Exercise struct {
	db DBTX
}

// NewExercise は Exercise を作る。
func NewExercise(db DBTX) *Exercise {
	return &Exercise{db: db}
}

// ExerciseFilter は一覧の絞り込み条件。
type ExerciseFilter struct {
	// MuscleGroup が nil なら部位で絞らない
	MuscleGroup *openapi.MuscleGroup

	// IncludeDeleted は論理削除済みを含めるか。同期用途以外では false（openapi.yaml）
	IncludeDeleted bool
}

// 列の順序は scanExercise と一致させること
const exerciseColumns = `id, name, muscle_group, is_compound, default_rest_sec,
	created_at, updated_at, deleted_at`

// List は条件に合う種目を返す。名前順。
func (r *Exercise) List(ctx context.Context, f ExerciseFilter) ([]openapi.Exercise, error) {
	// $1 が nil のときは部位で絞らない、を SQL 側で表現する。
	// Go 側で文字列を組み立てると条件が増えるたびに分岐が増える
	const q = `
		select ` + exerciseColumns + `
		from exercises
		where ($1::muscle_group is null or muscle_group = $1::muscle_group)
		  and ($2::boolean or deleted_at is null)
		order by name`

	var mg *string
	if f.MuscleGroup != nil {
		s := string(*f.MuscleGroup)
		mg = &s
	}

	rows, err := r.db.Query(ctx, q, mg, f.IncludeDeleted)
	if err != nil {
		return nil, fmt.Errorf("種目の一覧を引けない: %w", err)
	}
	defer rows.Close()

	items := make([]openapi.Exercise, 0, 64)
	for rows.Next() {
		e, err := scanExercise(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("種目の一覧を読めない: %w", err)
	}

	return items, nil
}

// Get は id の種目を返す。論理削除済みも返す（削除したことを確認できるようにするため）。
func (r *Exercise) Get(ctx context.Context, id uuid.UUID) (openapi.Exercise, error) {
	const q = `select ` + exerciseColumns + ` from exercises where id = $1`

	e, err := scanExercise(r.db.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.Exercise{}, fmt.Errorf("種目 %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return openapi.Exercise{}, err
	}

	return e, nil
}

// SoftDelete は deleted_at を立てる。物理削除しない（ADR-0014）。
func (r *Exercise) SoftDelete(ctx context.Context, id uuid.UUID) error {
	const q = `
		update exercises
		set deleted_at = $2, updated_at = $2
		where id = $1 and deleted_at is null`

	tag, err := r.db.Exec(ctx, q, id, timeutil.Now())
	if err != nil {
		return fmt.Errorf("種目を削除できない: %w", err)
	}
	// 0 件は「存在しない」か「既に削除済み」。どちらもクライアントには 404 でよい
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("種目 %s: %w", id, ErrNotFound)
	}

	return nil
}

// row は pgx.Row と pgx.Rows のどちらでも受けるための最小インターフェース。
type row interface {
	Scan(dest ...any) error
}

func scanExercise(r row) (openapi.Exercise, error) {
	var (
		e  openapi.Exercise
		mg string
	)
	err := r.Scan(&e.Id, &e.Name, &mg, &e.IsCompound, &e.DefaultRestSec,
		&e.CreatedAt, &e.UpdatedAt, &e.DeletedAt)
	if err != nil {
		return openapi.Exercise{}, err
	}

	e.MuscleGroup = openapi.MuscleGroup(mg)
	// DB は timestamptz を UTC で返す。表示・比較は JST に揃える（ADR-0013）
	e.CreatedAt = e.CreatedAt.In(timeutil.JST)
	e.UpdatedAt = e.UpdatedAt.In(timeutil.JST)
	if e.DeletedAt != nil {
		jst := e.DeletedAt.In(timeutil.JST)
		e.DeletedAt = &jst
	}

	return e, nil
}

// ExerciseInput は種目の作成・更新の入力。
type ExerciseInput struct {
	// ID はクライアントが生成した UUID。nil ならサーバが採番する。
	// 指定された場合、同じ ID での再送は冪等に扱う（オフライン入力の再送のため）
	ID *uuid.UUID

	Name        string
	MuscleGroup openapi.MuscleGroup
	IsCompound  *bool

	// DefaultRestSec は nil なら未設定
	DefaultRestSec *int
}

// Create は種目を作る。ID 指定で再送された場合は既存を返す（冪等）。
func (r *Exercise) Create(ctx context.Context, in ExerciseInput) (openapi.Exercise, error) {
	// on conflict (id) do nothing + returning だと、衝突時に0行になる。
	// 冪等にするため、0行なら既存を読み直す
	const q = `
		insert into exercises (id, name, muscle_group, is_compound, default_rest_sec)
		values (coalesce($1, gen_random_uuid()), $2, $3, coalesce($4, false), $5)
		on conflict (id) do nothing
		returning ` + exerciseColumns

	isCompound := false
	if in.IsCompound != nil {
		isCompound = *in.IsCompound
	}

	e, err := scanExercise(r.db.QueryRow(ctx, q, in.ID, in.Name, string(in.MuscleGroup), isCompound, in.DefaultRestSec))
	switch {
	case errors.Is(err, pgx.ErrNoRows) && in.ID != nil:
		// 同じ id での再送。既存をそのまま返す
		return r.Get(ctx, *in.ID)
	case isUniqueViolation(err):
		return openapi.Exercise{}, fmt.Errorf("種目名 %q: %w", in.Name, ErrConflict)
	case err != nil:
		return openapi.Exercise{}, fmt.Errorf("種目を作れない: %w", err)
	}

	return e, nil
}

// Update は種目を更新する。論理削除済みは対象外。
func (r *Exercise) Update(ctx context.Context, id uuid.UUID, in ExerciseInput) (openapi.Exercise, error) {
	const q = `
		update exercises
		set name = $2, muscle_group = $3,
		    is_compound = coalesce($4, is_compound),
		    default_rest_sec = $5,
		    updated_at = $6
		where id = $1 and deleted_at is null
		returning ` + exerciseColumns

	// updated_at はサーバ時刻で進める。競合解決に使うので、
	// クライアントの時計に依存させない（ADR-0014）
	e, err := scanExercise(r.db.QueryRow(ctx, q,
		id, in.Name, string(in.MuscleGroup), in.IsCompound, in.DefaultRestSec, timeutil.Now()))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return openapi.Exercise{}, fmt.Errorf("種目 %s: %w", id, ErrNotFound)
	case isUniqueViolation(err):
		return openapi.Exercise{}, fmt.Errorf("種目名 %q: %w", in.Name, ErrConflict)
	case err != nil:
		return openapi.Exercise{}, fmt.Errorf("種目を更新できない: %w", err)
	}

	return e, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}
