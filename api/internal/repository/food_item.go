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

// FoodItem は食品マスタへのアクセス（要件 N-02 / ADR-0017）。
type FoodItem struct {
	db DBTX
}

// NewFoodItem は FoodItem を作る。
func NewFoodItem(db DBTX) *FoodItem {
	return &FoodItem{db: db}
}

// 列の順序は scanFoodItem と一致させること
const foodItemColumns = `id, name, qty, protein_g, fat_g, carb_g,
	used_count, created_at, updated_at, deleted_at`

// List は食品マスタを返す。**よく使う順。**
//
// q が空でなければ名前で絞る。
func (r *FoodItem) List(ctx context.Context, q string) ([]openapi.FoodItem, error) {
	// $1 が空のときは絞らない、を SQL 側で表現する
	const query = `
		select ` + foodItemColumns + `
		from food_items
		where deleted_at is null
		  and ($1 = '' or name ilike '%' || $1 || '%')
		order by used_count desc, last_used_at desc nulls last, name`

	rows, err := r.db.Query(ctx, query, q)
	if err != nil {
		return nil, fmt.Errorf("食品マスタを引けない: %w", err)
	}
	defer rows.Close()

	items := make([]openapi.FoodItem, 0, 32)
	ids := make([]uuid.UUID, 0, 32)
	for rows.Next() {
		it, err := scanFoodItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
		ids = append(ids, it.Id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("食品マスタを読めない: %w", err)
	}

	// **1件ずつ引かない。** 項目が増えるとクエリ数が比例して増える
	byItem, err := r.componentsOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range items {
		// **map から取ると無いキーは nil。** null を返さないよう空配列に揃える
		// （引数なしの項目の方が多いので、ここが既定の経路）
		items[i].Components = emptyIfNil(byItem[items[i].Id])
	}

	return items, nil
}

// Get は1件返す。
func (r *FoodItem) Get(ctx context.Context, id uuid.UUID) (openapi.FoodItem, error) {
	const q = `select ` + foodItemColumns + ` from food_items where id = $1 and deleted_at is null`

	it, err := scanFoodItem(r.db.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.FoodItem{}, fmt.Errorf("食品 %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return openapi.FoodItem{}, fmt.Errorf("食品を取得できない: %w", err)
	}

	byItem, err := r.componentsOf(ctx, []uuid.UUID{id})
	if err != nil {
		return openapi.FoodItem{}, err
	}
	it.Components = emptyIfNil(byItem[id])

	return it, nil
}

// Create は登録する。
func (r *FoodItem) Create(ctx context.Context, in openapi.FoodItemInput) (openapi.FoodItem, error) {
	const q = `
		insert into food_items (name, qty, protein_g, fat_g, carb_g)
		values ($1, $2, $3, $4, $5)
		returning ` + foodItemColumns

	it, err := scanFoodItem(r.db.QueryRow(ctx, q,
		in.Name, in.Qty, in.ProteinG, in.FatG, in.CarbG))
	if err != nil {
		return openapi.FoodItem{}, fmt.Errorf("食品を登録できない: %w", err)
	}

	if err := r.replaceComponents(ctx, it.Id, in.Components); err != nil {
		return openapi.FoodItem{}, err
	}

	return r.Get(ctx, it.Id)
}

// Update は直す。**構成はまるごと置き換わる。**
func (r *FoodItem) Update(ctx context.Context, id uuid.UUID, in openapi.FoodItemInput) (openapi.FoodItem, error) {
	const q = `
		update food_items set name = $2, qty = $3,
			protein_g = $4, fat_g = $5, carb_g = $6, updated_at = $7
		where id = $1 and deleted_at is null
		returning ` + foodItemColumns

	_, err := scanFoodItem(r.db.QueryRow(ctx, q,
		id, in.Name, in.Qty, in.ProteinG, in.FatG, in.CarbG, timeutil.Now()))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.FoodItem{}, fmt.Errorf("食品 %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return openapi.FoodItem{}, fmt.Errorf("食品を更新できない: %w", err)
	}

	if err := r.replaceComponents(ctx, id, in.Components); err != nil {
		return openapi.FoodItem{}, err
	}

	return r.Get(ctx, id)
}

// Delete は論理削除する（ADR-0014）。
func (r *FoodItem) Delete(ctx context.Context, id uuid.UUID) error {
	const q = `update food_items set deleted_at = $2, updated_at = $2
		where id = $1 and deleted_at is null`

	tag, err := r.db.Exec(ctx, q, id, timeutil.Now())
	if err != nil {
		return fmt.Errorf("食品を削除できない: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("食品 %s: %w", id, ErrNotFound)
	}

	return nil
}

// MarkUsed は使った回数を1つ増やす。一覧の並び順に効く。
func (r *FoodItem) MarkUsed(ctx context.Context, id uuid.UUID) error {
	const q = `update food_items set used_count = used_count + 1, last_used_at = $2
		where id = $1 and deleted_at is null`

	if _, err := r.db.Exec(ctx, q, id, timeutil.Now()); err != nil {
		return fmt.Errorf("使用回数を更新できない: %w", err)
	}

	return nil
}

// replaceComponents は構成を入れ替える。
//
// **差分更新にしない。** 「消したつもりが残っている」が起きる（plan と同じ判断）。
func (r *FoodItem) replaceComponents(ctx context.Context, id uuid.UUID, in *[]openapi.FoodItemComponent) error {
	if _, err := r.db.Exec(ctx, `delete from food_item_components where food_item_id = $1`, id); err != nil {
		return fmt.Errorf("構成を消せない: %w", err)
	}
	if in == nil || len(*in) == 0 {
		return nil
	}

	const q = `
		insert into food_item_components
			(food_item_id, item_order, name, unit, basis_amount, default_amount,
			 protein_g, fat_g, carb_g)
		values ($1, $2, $3, coalesce($4, 'g'), $5, $6,
			-- **リテラルの 0 に型を付ける。** QueryExecModeExec では
			-- pgx がサーバに型を問い合わせないので、0 が integer と
			-- 解釈されて "17.5" が入らなくなる
			coalesce($7, 0::numeric), coalesce($8, 0::numeric), coalesce($9, 0::numeric))`

	for i, c := range *in {
		_, err := r.db.Exec(ctx, q, id, i+1, c.Name, c.Unit,
			c.BasisAmount, c.DefaultAmount, c.ProteinG, c.FatG, c.CarbG)
		if err != nil {
			return fmt.Errorf("構成を保存できない（%s）: %w", c.Name, err)
		}
	}

	return nil
}

// componentsOf は複数の項目の構成をまとめて引く。
func (r *FoodItem) componentsOf(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]openapi.FoodItemComponent, error) {
	out := map[uuid.UUID][]openapi.FoodItemComponent{}
	if len(ids) == 0 {
		return out, nil
	}

	// **`any($1)` に []uuid.UUID を渡さない。** 本番は
	// QueryExecModeExec（Supavisor の transaction mode 向け）で動いており、
	// プリペアドを使わないので pgx が要素の型を解決できない。
	// 文字列にして `::uuid[]` で明示する
	list := make([]string, 0, len(ids))
	for _, id := range ids {
		list = append(list, id.String())
	}

	const q = `
		select food_item_id, name, unit, basis_amount, default_amount,
			protein_g, fat_g, carb_g
		from food_item_components
		where food_item_id = any($1::uuid[])
		order by food_item_id, item_order`

	rows, err := r.db.Query(ctx, q, list)
	if err != nil {
		return nil, fmt.Errorf("構成を引けない: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var itemID uuid.UUID
		var c openapi.FoodItemComponent
		var unit string
		var p, f, cb float32
		if err := rows.Scan(&itemID, &c.Name, &unit, &c.BasisAmount, &c.DefaultAmount,
			&p, &f, &cb); err != nil {
			return nil, fmt.Errorf("構成を読めない: %w", err)
		}
		c.Unit, c.ProteinG, c.FatG, c.CarbG = &unit, &p, &f, &cb
		out[itemID] = append(out[itemID], c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("構成を読めない: %w", err)
	}

	return out, nil
}

// emptyIfNil は nil を空配列にする。**クライアントが map する前提。**
func emptyIfNil(cs []openapi.FoodItemComponent) []openapi.FoodItemComponent {
	if cs == nil {
		return []openapi.FoodItemComponent{}
	}

	return cs
}

func scanFoodItem(row pgx.Row) (openapi.FoodItem, error) {
	var it openapi.FoodItem
	err := row.Scan(&it.Id, &it.Name, &it.Qty, &it.ProteinG, &it.FatG, &it.CarbG,
		&it.UsedCount, &it.CreatedAt, &it.UpdatedAt, &it.DeletedAt)
	if err != nil {
		return openapi.FoodItem{}, err
	}
	// null ではなく空配列を返す。クライアントが map する前提（handler と同じ理由）
	it.Components = []openapi.FoodItemComponent{}

	return it, nil
}
