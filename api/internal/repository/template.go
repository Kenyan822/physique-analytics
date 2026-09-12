package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// Template はトレーニングテンプレートへのアクセス。
type Template struct {
	db DBTX
}

// NewTemplate は Template を作る。
func NewTemplate(db DBTX) *Template {
	return &Template{db: db}
}

// TemplateInput はテンプレートの作成・更新の入力。
type TemplateInput struct {
	ID    *uuid.UUID
	Name  string
	Items []openapi.TemplateItem
}

const templateColumns = `id, name, created_at, updated_at, deleted_at`

const templateItemColumns = `exercise_id, item_order, target_sets,
	target_reps_min, target_reps_max, target_rir`

// List はテンプレートを名前順に返す。論理削除済みは含まない。
func (r *Template) List(ctx context.Context) ([]openapi.Template, error) {
	const q = `select ` + templateColumns + `
		from templates where deleted_at is null order by name`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("テンプレートの一覧を引けない: %w", err)
	}
	defer rows.Close()

	out := make([]openapi.Template, 0, 16)
	for rows.Next() {
		tpl, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, tpl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("テンプレートの一覧を読めない: %w", err)
	}

	for i := range out {
		out[i].Items, err = r.listItems(ctx, out[i].Id)
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

// Get はテンプレートを1件返す。
func (r *Template) Get(ctx context.Context, id uuid.UUID) (openapi.Template, error) {
	const q = `select ` + templateColumns + `
		from templates where id = $1 and deleted_at is null`

	tpl, err := scanTemplate(r.db.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.Template{}, fmt.Errorf("テンプレート %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return openapi.Template{}, fmt.Errorf("テンプレートを取得できない: %w", err)
	}

	tpl.Items, err = r.listItems(ctx, id)
	if err != nil {
		return openapi.Template{}, err
	}

	return tpl, nil
}

// Create はテンプレートを作る。ID 指定で既存なら冪等に既存を返す。
func (r *Template) Create(ctx context.Context, in TemplateInput) (openapi.Template, error) {
	const q = `
		insert into templates (id, name)
		values (coalesce($1, gen_random_uuid()), $2)
		on conflict (id) do nothing
		returning ` + templateColumns

	tpl, err := scanTemplate(r.db.QueryRow(ctx, q, in.ID, in.Name))
	switch {
	case errors.Is(err, pgx.ErrNoRows) && in.ID != nil:
		return r.Get(ctx, *in.ID)
	case err != nil:
		return openapi.Template{}, fmt.Errorf("テンプレートを作れない: %w", err)
	}

	if err := r.replaceItems(ctx, tpl.Id, in.Items); err != nil {
		return openapi.Template{}, err
	}
	tpl.Items, err = r.listItems(ctx, tpl.Id)
	if err != nil {
		return openapi.Template{}, err
	}

	return tpl, nil
}

// Update はテンプレートを更新する。項目は全入れ替え。
func (r *Template) Update(ctx context.Context, id uuid.UUID, in TemplateInput) (openapi.Template, error) {
	const q = `
		update templates set name = $2, updated_at = $3
		where id = $1 and deleted_at is null
		returning ` + templateColumns

	tpl, err := scanTemplate(r.db.QueryRow(ctx, q, id, in.Name, timeutil.Now()))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.Template{}, fmt.Errorf("テンプレート %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return openapi.Template{}, fmt.Errorf("テンプレートを更新できない: %w", err)
	}

	// 項目は差分更新せず全入れ替えする。order の付け替えを差分で扱うと
	// 一意制約に引っかかる順序が出てくるうえ、実運用では毎回作り直しに近い
	if err := r.replaceItems(ctx, id, in.Items); err != nil {
		return openapi.Template{}, err
	}
	tpl.Items, err = r.listItems(ctx, id)
	if err != nil {
		return openapi.Template{}, err
	}

	return tpl, nil
}

// SoftDelete はテンプレートを論理削除する（ADR-0014）。
func (r *Template) SoftDelete(ctx context.Context, id uuid.UUID) error {
	const q = `
		update templates set deleted_at = $2, updated_at = $2
		where id = $1 and deleted_at is null`

	tag, err := r.db.Exec(ctx, q, id, timeutil.Now())
	if err != nil {
		return fmt.Errorf("テンプレートを削除できない: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("テンプレート %s: %w", id, ErrNotFound)
	}

	return nil
}

func (r *Template) replaceItems(ctx context.Context, id uuid.UUID, items []openapi.TemplateItem) error {
	// template_items は論理削除しない。テンプレートは「今の献立」であって
	// 記録ではないので、過去の構成を残す意味が無い（セッションが参照するのは
	// templates.id であって項目ではない）
	if _, err := r.db.Exec(ctx, `delete from template_items where template_id = $1`, id); err != nil {
		return fmt.Errorf("テンプレートの項目を消せない: %w", err)
	}

	const q = `
		insert into template_items
			(template_id, exercise_id, item_order, target_sets, target_reps_min, target_reps_max, target_rir)
		values ($1, $2, $3, $4, $5, $6, $7)`

	for _, it := range items {
		_, err := r.db.Exec(ctx, q, id, it.ExerciseId, it.Order, it.TargetSets,
			it.TargetRepsMin, it.TargetRepsMax, it.TargetRir)
		if isUniqueViolation(err) {
			return fmt.Errorf("order %d が重複している: %w", it.Order, ErrConflict)
		}
		if err != nil {
			return fmt.Errorf("テンプレートの項目を作れない: %w", err)
		}
	}

	return nil
}

func (r *Template) listItems(ctx context.Context, id uuid.UUID) ([]openapi.TemplateItem, error) {
	const q = `select ` + templateItemColumns + `
		from template_items where template_id = $1 order by item_order`

	rows, err := r.db.Query(ctx, q, id)
	if err != nil {
		return nil, fmt.Errorf("テンプレートの項目を引けない: %w", err)
	}
	defer rows.Close()

	items := make([]openapi.TemplateItem, 0, 8)
	for rows.Next() {
		var it openapi.TemplateItem
		err := rows.Scan(&it.ExerciseId, &it.Order, &it.TargetSets,
			&it.TargetRepsMin, &it.TargetRepsMax, &it.TargetRir)
		if err != nil {
			return nil, fmt.Errorf("テンプレートの項目を読めない: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("テンプレートの項目を読めない: %w", err)
	}

	return items, nil
}

func scanTemplate(r row) (openapi.Template, error) {
	var tpl openapi.Template
	if err := r.Scan(&tpl.Id, &tpl.Name, &tpl.CreatedAt, &tpl.UpdatedAt, &tpl.DeletedAt); err != nil {
		return openapi.Template{}, err
	}

	tpl.CreatedAt = tpl.CreatedAt.In(timeutil.JST)
	tpl.UpdatedAt = tpl.UpdatedAt.In(timeutil.JST)
	if tpl.DeletedAt != nil {
		jst := tpl.DeletedAt.In(timeutil.JST)
		tpl.DeletedAt = &jst
	}
	tpl.Items = []openapi.TemplateItem{}

	return tpl, nil
}
