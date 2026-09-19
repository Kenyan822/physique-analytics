package repository

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// Meal は食事記録へのアクセス（要件 N-01 / N-02 / N-04）。
//
// **食品マスタを持たない。** 記録時に名前と PFC を直接入れ、過去の記録が
// そのまま候補になる（docs/01-要件定義.md §4.3）。
type Meal struct {
	db DBTX
}

// NewMeal は Meal を作る。
func NewMeal(db DBTX) *Meal {
	return &Meal{db: db}
}

// MealInput は食事の作成・更新の入力。
type MealInput struct {
	// ID はクライアント生成の UUID。nil ならサーバが採番する
	ID   *uuid.UUID
	Date openapi_types.Date
	// At は "HH:MM"。nil は「時刻を記録していない」
	At   *string
	Slot *openapi.MealSlot
	// Name は nil で「記録していない」。PFC だけの記録を許す（#188）
	Name     *string
	Qty      *string
	Kcal     *int
	ProteinG *float32
	FatG     *float32
	CarbG    *float32
	// Source は未指定なら manual
	Source *openapi.MealSource
}

// **eaten_at は to_char で HH:MM に落とす。** time 型をそのまま返すと
// "19:40:00" になり、API が約束している形と食い違う。
// pgtype を挟むより SQL 側で固定する方が、経路が1つで済む
const mealColumns = `id, date, to_char(eaten_at, 'HH24:MI'), slot, name, qty,
	kcal, protein_g, fat_g, carb_g,
	source, created_at, updated_at, deleted_at`

// List は期間内の食事を日付順に返す。
func (r *Meal) List(ctx context.Context, from, to *openapi_types.Date) ([]openapi.Meal, error) {
	const q = `
		select ` + mealColumns + `
		from meals
		where deleted_at is null
		  and ($1::date is null or date >= $1::date)
		  and ($2::date is null or date <= $2::date)
		-- 時刻順に並べる。持っていない既存の記録は後ろへ（nulls last）
		order by date desc, eaten_at nulls last, created_at`

	rows, err := r.db.Query(ctx, q, dateOrNil(from), dateOrNil(to))
	if err != nil {
		return nil, fmt.Errorf("食事の一覧を引けない: %w", err)
	}
	defer rows.Close()

	out := make([]openapi.Meal, 0, 32)
	for rows.Next() {
		m, err := scanMeal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("食事の一覧を読めない: %w", err)
	}

	return out, nil
}

// Get は食事を1件返す。
func (r *Meal) Get(ctx context.Context, id uuid.UUID) (openapi.Meal, error) {
	const q = `select ` + mealColumns + ` from meals where id = $1 and deleted_at is null`

	m, err := scanMeal(r.db.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.Meal{}, fmt.Errorf("食事 %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return openapi.Meal{}, fmt.Errorf("食事を取得できない: %w", err)
	}

	return m, nil
}

// Create は食事を記録する。ID 指定で既存なら冪等に既存を返す。
func (r *Meal) Create(ctx context.Context, in MealInput) (openapi.Meal, error) {
	const q = `
		insert into meals (id, date, eaten_at, slot, name, qty,
			kcal, protein_g, fat_g, carb_g, source)
		values (coalesce($1, gen_random_uuid()), $2, $3::time, $4, $5, $6, $7, $8, $9, $10,
		        coalesce($11, 'manual'))
		on conflict (id) do nothing
		returning ` + mealColumns

	m, err := scanMeal(r.db.QueryRow(ctx, q,
		in.ID, in.Date.Time, in.At, slotOrNil(in.Slot), in.Name, in.Qty,
		in.Kcal, in.ProteinG, in.FatG, in.CarbG, sourceOrNil(in.Source)))
	switch {
	// オフラインからの再送で二重に入らないようにする（要件 T-07 と同じ理由）
	case errors.Is(err, pgx.ErrNoRows) && in.ID != nil:
		return r.Get(ctx, *in.ID)
	case err != nil:
		return openapi.Meal{}, fmt.Errorf("食事を記録できない: %w", err)
	}

	return m, nil
}

// Update は食事を更新する。
func (r *Meal) Update(ctx context.Context, id uuid.UUID, in MealInput) (openapi.Meal, error) {
	const q = `
		update meals set date = $2, eaten_at = $3::time, slot = $4, name = $5, qty = $6,
			kcal = $7, protein_g = $8, fat_g = $9, carb_g = $10,
			source = coalesce($11, source), updated_at = $12
		where id = $1 and deleted_at is null
		returning ` + mealColumns

	m, err := scanMeal(r.db.QueryRow(ctx, q,
		id, in.Date.Time, in.At, slotOrNil(in.Slot), in.Name, in.Qty,
		in.Kcal, in.ProteinG, in.FatG, in.CarbG, sourceOrNil(in.Source), timeutil.Now()))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.Meal{}, fmt.Errorf("食事 %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return openapi.Meal{}, fmt.Errorf("食事を更新できない: %w", err)
	}

	return m, nil
}

// SoftDelete は食事を論理削除する（ADR-0014）。
func (r *Meal) SoftDelete(ctx context.Context, id uuid.UUID) error {
	const q = `update meals set deleted_at = $2, updated_at = $2
		where id = $1 and deleted_at is null`

	tag, err := r.db.Exec(ctx, q, id, timeutil.Now())
	if err != nil {
		return fmt.Errorf("食事を削除できない: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("食事 %s: %w", id, ErrNotFound)
	}

	return nil
}

// Copy は fromDate の食事を toDate に複製する（要件 N-04）。
//
// **新しい ID で作る。** 同じ ID で日付だけ変えると、元の記録が移動してしまう。
func (r *Meal) Copy(ctx context.Context, from, to openapi_types.Date, slot *openapi.MealSlot) ([]openapi.Meal, error) {
	const q = `
		insert into meals (date, slot, name, qty, kcal, protein_g, fat_g, carb_g, source)
		select $2::date, slot, name, qty, kcal, protein_g, fat_g, carb_g, source
		from meals
		where deleted_at is null and date = $1::date
		  and ($3::text is null or slot = $3::text)
		order by created_at
		returning ` + mealColumns

	rows, err := r.db.Query(ctx, q, from.Time, to.Time, slotOrNil(slot))
	if err != nil {
		return nil, fmt.Errorf("食事を複製できない: %w", err)
	}
	defer rows.Close()

	out := make([]openapi.Meal, 0, 8)
	for rows.Next() {
		m, err := scanMeal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("複製した食事を読めない: %w", err)
	}

	return out, nil
}

// Suggestions は過去の記録から候補を頻度順に返す（要件 N-02）。
//
// 直近の PFC を一緒に返すので、選んだ時点で入力が終わる。
// 同じ名前で内容が変わることはあるが、**最後に記録した値が一番近い**。
func (r *Meal) Suggestions(ctx context.Context, query string, limit int) ([]openapi.MealSuggestion, error) {
	const q = `
		select distinct on (m.name)
			m.name, c.cnt, m.date, m.qty, m.kcal, m.protein_g, m.fat_g, m.carb_g
		from meals m
		join (
			select name, count(*) as cnt
			from meals where deleted_at is null
			group by name
		) c on c.name = m.name
		where m.deleted_at is null
		  and ($1::text = '' or m.name like '%' || $1 || '%')
		order by m.name, m.date desc, m.created_at desc`

	rows, err := r.db.Query(ctx, q, query)
	if err != nil {
		return nil, fmt.Errorf("候補を引けない: %w", err)
	}
	defer rows.Close()

	out := make([]openapi.MealSuggestion, 0, limit)
	for rows.Next() {
		var s openapi.MealSuggestion
		var date openapi_types.Date
		if err := rows.Scan(&s.Name, &s.Count, &date.Time, &s.Qty,
			&s.Kcal, &s.ProteinG, &s.FatG, &s.CarbG); err != nil {
			return nil, fmt.Errorf("候補を読めない: %w", err)
		}
		s.LastDate = &date
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("候補を読めない: %w", err)
	}

	// distinct on は name 順でしか並べられないので、件数順は Go 側で付ける
	sortByCountDesc(out)
	if len(out) > limit {
		out = out[:limit]
	}

	return out, nil
}

func sortByCountDesc(items []openapi.MealSuggestion) {
	// 件数が同じなら直近に食べた方を先に出す
	slices.SortStableFunc(items, func(a, b openapi.MealSuggestion) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		if a.LastDate != nil && b.LastDate != nil && !a.LastDate.Equal(b.LastDate.Time) {
			if a.LastDate.After(b.LastDate.Time) {
				return -1
			}

			return 1
		}

		return 0
	})
}

func scanMeal(row pgx.Row) (openapi.Meal, error) {
	var m openapi.Meal
	var slot, source *string
	err := row.Scan(&m.Id, &m.Date.Time, &m.At, &slot, &m.Name, &m.Qty,
		&m.Kcal, &m.ProteinG, &m.FatG, &m.CarbG, &source,
		&m.CreatedAt, &m.UpdatedAt, &m.DeletedAt)
	if err != nil {
		return openapi.Meal{}, err
	}
	if slot != nil {
		s := openapi.MealSlot(*slot)
		m.Slot = &s
	}
	if source != nil {
		m.Source = openapi.MealSource(*source)
	}

	return m, nil
}

func slotOrNil(s *openapi.MealSlot) *string {
	if s == nil {
		return nil
	}
	v := string(*s)

	return &v
}

func sourceOrNil(s *openapi.MealSource) *string {
	if s == nil {
		return nil
	}
	v := string(*s)

	return &v
}
