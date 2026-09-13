package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// MealSet は食事セットへのアクセス（要件 N-03）。
//
// 「朝食セット」のように毎回同じ組み合わせで食べるものを保存しておき、
// ワンタップで記録できるようにする。**食品マスタではない。**
type MealSet struct {
	db DBTX
}

// NewMealSet は MealSet を作る。
func NewMealSet(db DBTX) *MealSet {
	return &MealSet{db: db}
}

const mealSetColumns = `id, name, slot, created_at, updated_at, deleted_at`

const mealSetItemColumns = `name, qty, kcal, protein_g, fat_g, carb_g`

// List は食事セットを名前順に返す。
func (r *MealSet) List(ctx context.Context) ([]openapi.MealSet, error) {
	const q = `select ` + mealSetColumns + ` from meal_sets
		where deleted_at is null order by name`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("食事セットの一覧を引けない: %w", err)
	}
	defer rows.Close()

	out := make([]openapi.MealSet, 0, 16)
	for rows.Next() {
		s, err := scanMealSet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("食事セットの一覧を読めない: %w", err)
	}

	for i := range out {
		if out[i].Items, err = r.listItems(ctx, out[i].Id); err != nil {
			return nil, err
		}
	}

	return out, nil
}

// Get は食事セットを1件返す。
func (r *MealSet) Get(ctx context.Context, id uuid.UUID) (openapi.MealSet, error) {
	const q = `select ` + mealSetColumns + ` from meal_sets
		where id = $1 and deleted_at is null`

	s, err := scanMealSet(r.db.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.MealSet{}, fmt.Errorf("食事セット %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return openapi.MealSet{}, fmt.Errorf("食事セットを取得できない: %w", err)
	}

	if s.Items, err = r.listItems(ctx, id); err != nil {
		return openapi.MealSet{}, err
	}

	return s, nil
}

// Create は食事セットを作る。同じ名前があれば ErrConflict。
func (r *MealSet) Create(ctx context.Context, in openapi.MealSetInput) (openapi.MealSet, error) {
	const q = `insert into meal_sets (name, slot) values ($1, $2)
		returning ` + mealSetColumns

	s, err := scanMealSet(r.db.QueryRow(ctx, q, in.Name, slotOrNil(in.Slot)))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return openapi.MealSet{}, fmt.Errorf("食事セット %q: %w", in.Name, ErrConflict)
	}
	if err != nil {
		return openapi.MealSet{}, fmt.Errorf("食事セットを作れない: %w", err)
	}

	if err := r.replaceItems(ctx, s.Id, in.Items); err != nil {
		return openapi.MealSet{}, err
	}
	if s.Items, err = r.listItems(ctx, s.Id); err != nil {
		return openapi.MealSet{}, err
	}

	return s, nil
}

// Update は食事セットを更新する。項目は全入れ替え。
//
// 差分更新にしない。item_order の付け替えを差分で扱うと一意制約に
// 引っかかる順序が出てくるうえ、実運用では毎回作り直しに近い。
func (r *MealSet) Update(ctx context.Context, id uuid.UUID, in openapi.MealSetInput) (openapi.MealSet, error) {
	const q = `update meal_sets set name = $2, slot = $3, updated_at = $4
		where id = $1 and deleted_at is null
		returning ` + mealSetColumns

	s, err := scanMealSet(r.db.QueryRow(ctx, q, id, in.Name, slotOrNil(in.Slot), timeutil.Now()))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.MealSet{}, fmt.Errorf("食事セット %s: %w", id, ErrNotFound)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return openapi.MealSet{}, fmt.Errorf("食事セット %q: %w", in.Name, ErrConflict)
	}
	if err != nil {
		return openapi.MealSet{}, fmt.Errorf("食事セットを更新できない: %w", err)
	}

	if err := r.replaceItems(ctx, id, in.Items); err != nil {
		return openapi.MealSet{}, err
	}
	if s.Items, err = r.listItems(ctx, id); err != nil {
		return openapi.MealSet{}, err
	}

	return s, nil
}

// SoftDelete は食事セットを論理削除する（ADR-0014）。
func (r *MealSet) SoftDelete(ctx context.Context, id uuid.UUID) error {
	const q = `update meal_sets set deleted_at = $2, updated_at = $2
		where id = $1 and deleted_at is null`

	tag, err := r.db.Exec(ctx, q, id, timeutil.Now())
	if err != nil {
		return fmt.Errorf("食事セットを削除できない: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("食事セット %s: %w", id, ErrNotFound)
	}

	return nil
}

// Apply はセットの内容をその日の meals に写す（要件 N-03）。
//
// **写したあとは個別に編集・削除できる。** 実際に食べた量は日によって
// 変わるので、セットへの参照ではなく実体を作る。
func (r *MealSet) Apply(ctx context.Context, id uuid.UUID, date openapi_types.Date, slot *openapi.MealSlot) ([]openapi.Meal, error) {
	set, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// 指定が無ければセットの既定の区分を使う
	use := slot
	if use == nil {
		use = set.Slot
	}

	const q = `
		insert into meals (date, slot, name, qty, kcal, protein_g, fat_g, carb_g)
		select $1::date, $2::text, i.name, i.qty, i.kcal, i.protein_g, i.fat_g, i.carb_g
		from meal_set_items i
		where i.meal_set_id = $3
		order by i.item_order
		returning ` + mealColumns

	rows, err := r.db.Query(ctx, q, date.Time, slotOrNil(use), id)
	if err != nil {
		return nil, fmt.Errorf("食事セットを展開できない: %w", err)
	}
	defer rows.Close()

	out := make([]openapi.Meal, 0, len(set.Items))
	for rows.Next() {
		m, err := scanMeal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("展開した食事を読めない: %w", err)
	}

	return out, nil
}

func (r *MealSet) listItems(ctx context.Context, id uuid.UUID) ([]openapi.MealSetItem, error) {
	const q = `select ` + mealSetItemColumns + ` from meal_set_items
		where meal_set_id = $1 order by item_order`

	rows, err := r.db.Query(ctx, q, id)
	if err != nil {
		return nil, fmt.Errorf("食事セットの項目を引けない: %w", err)
	}
	defer rows.Close()

	out := make([]openapi.MealSetItem, 0, 8)
	for rows.Next() {
		var i openapi.MealSetItem
		if err := rows.Scan(&i.Name, &i.Qty, &i.Kcal, &i.ProteinG, &i.FatG, &i.CarbG); err != nil {
			return nil, fmt.Errorf("食事セットの項目を読めない: %w", err)
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("食事セットの項目を読めない: %w", err)
	}

	return out, nil
}

func (r *MealSet) replaceItems(ctx context.Context, id uuid.UUID, items []openapi.MealSetItem) error {
	// 項目は論理削除しない。セットは「よく食べる組み合わせ」であって
	// 記録ではないので、過去の構成を残す意味が無い
	if _, err := r.db.Exec(ctx, `delete from meal_set_items where meal_set_id = $1`, id); err != nil {
		return fmt.Errorf("食事セットの項目を消せない: %w", err)
	}

	const q = `
		insert into meal_set_items
			(meal_set_id, item_order, name, qty, kcal, protein_g, fat_g, carb_g)
		values ($1, $2, $3, $4, $5, $6, $7, $8)`
	for i, it := range items {
		if _, err := r.db.Exec(ctx, q, id, i+1, it.Name, it.Qty,
			it.Kcal, it.ProteinG, it.FatG, it.CarbG); err != nil {
			return fmt.Errorf("食事セットの項目 %q を保存できない: %w", it.Name, err)
		}
	}

	return nil
}

func scanMealSet(row pgx.Row) (openapi.MealSet, error) {
	var s openapi.MealSet
	var slot *string
	if err := row.Scan(&s.Id, &s.Name, &slot, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt); err != nil {
		return openapi.MealSet{}, err
	}
	if slot != nil {
		v := openapi.MealSlot(*slot)
		s.Slot = &v
	}
	s.Items = []openapi.MealSetItem{}

	return s, nil
}
