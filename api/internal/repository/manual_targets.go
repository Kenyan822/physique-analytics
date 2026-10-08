package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// ManualTargets は手で決めた摂取目標（要件 N-05）の履歴へのアクセス。
//
// **適用開始日つきの履歴。** その日の目標は `starts_on <= 日付` で最新の1行。
// active フラグは持たない（「最新の行」と同値で、矛盾した状態を作れてしまう。ADR-0018）
type ManualTargets struct {
	db DBTX
}

// NewManualTargets は ManualTargets を作る。
func NewManualTargets(db DBTX) *ManualTargets {
	return &ManualTargets{db: db}
}

const manualTargetCols = `id, name, starts_on, protein_g, fat_g, carb_g, created_at`

// On は date に適用される目標を返す。**無ければ nil**（エラーにしない）。
//
// 履歴が0件、または date が最初の開始日より前のとき nil になり、
// 呼び出し側は自動計算にフォールバックする。
func (r *ManualTargets) On(ctx context.Context, date time.Time) (*openapi.ManualTargets, error) {
	const q = `
		select ` + manualTargetCols + `
		from manual_targets
		where starts_on <= $1::date
		order by starts_on desc
		limit 1`

	// **date の暦日をそのまま渡す。** time.Time のままだと、接続のタイムゾーンで
	// 前日に丸められることがある
	e, err := scanManualTargetEntry(r.db.QueryRow(ctx, q, date.Format(time.DateOnly)))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("目標を取得できない: %w", err)
	}

	t := manualTargetsOf(e)

	return &t, nil
}

// List は履歴を開始日の新しい順に返す。
func (r *ManualTargets) List(ctx context.Context) ([]openapi.ManualTargetEntry, error) {
	const q = `select ` + manualTargetCols + ` from manual_targets order by starts_on desc`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("目標の履歴を取得できない: %w", err)
	}
	defer rows.Close()

	out := []openapi.ManualTargetEntry{}
	for rows.Next() {
		e, err := scanManualTargetEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("目標の履歴を読めない: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("目標の履歴を読めない: %w", err)
	}

	return out, nil
}

// Add は履歴に1件足す。**同じ開始日があれば上書き**する（starts_on は一意）。
func (r *ManualTargets) Add(ctx context.Context, in openapi.ManualTargetEntryInput) (openapi.ManualTargetEntry, error) {
	const q = `
		insert into manual_targets (name, starts_on, protein_g, fat_g, carb_g)
		values ($1, $2::date, $3, $4, $5)
		on conflict (starts_on) do update set
			name = excluded.name,
			protein_g = excluded.protein_g,
			fat_g = excluded.fat_g,
			carb_g = excluded.carb_g
		returning ` + manualTargetCols

	e, err := scanManualTargetEntry(r.db.QueryRow(ctx, q,
		in.Name, in.StartsOn.Format(time.DateOnly), in.ProteinG, in.FatG, in.CarbG))
	if err != nil {
		return openapi.ManualTargetEntry{}, fmt.Errorf("目標を保存できない: %w", err)
	}

	return e, nil
}

// Put は今日（JST）から適用する目標として履歴に足す。同じ日なら上書き。
//
// **名前には触れない。** 同じ日に Add で付けた名前を、名前を持たない Put が消さない
func (r *ManualTargets) Put(ctx context.Context, in openapi.ManualTargets) (openapi.ManualTargets, error) {
	const q = `
		insert into manual_targets (starts_on, protein_g, fat_g, carb_g)
		values ($1::date, $2, $3, $4)
		on conflict (starts_on) do update set
			protein_g = excluded.protein_g,
			fat_g = excluded.fat_g,
			carb_g = excluded.carb_g
		returning ` + manualTargetCols

	e, err := scanManualTargetEntry(r.db.QueryRow(ctx, q,
		timeutil.Now().Format(time.DateOnly), in.ProteinG, in.FatG, in.CarbG))
	if err != nil {
		return openapi.ManualTargets{}, fmt.Errorf("目標を保存できない: %w", err)
	}

	return manualTargetsOf(e), nil
}

// DeleteEntry は履歴の1件を消す。無ければ ErrNotFound。
func (r *ManualTargets) DeleteEntry(ctx context.Context, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `delete from manual_targets where id = $1`, id)
	if err != nil {
		return fmt.Errorf("目標を消せない: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// Delete は履歴を全部消す。**無くてもエラーにしない**（DELETE は冪等）。
func (r *ManualTargets) Delete(ctx context.Context) error {
	if _, err := r.db.Exec(ctx, `delete from manual_targets`); err != nil {
		return fmt.Errorf("目標を消せない: %w", err)
	}

	return nil
}

// scanManualTargetEntry は1行読む。
//
// **kcal は列に持たない。** PFC から計算できる（Atwater 4/9/4）。
// 持つと手入力と計算値が食い違ったときにどちらが正か決められなくなる
func scanManualTargetEntry(row pgx.Row) (openapi.ManualTargetEntry, error) {
	var e openapi.ManualTargetEntry
	if err := row.Scan(&e.Id, &e.Name, &e.StartsOn.Time, &e.ProteinG, &e.FatG, &e.CarbG, &e.CreatedAt); err != nil {
		return openapi.ManualTargetEntry{}, err
	}

	e.Kcal = analytics.KcalFromMacros(float64(e.ProteinG), float64(e.FatG), float64(e.CarbG))

	return e, nil
}

// manualTargetsOf は履歴の1行を、ある日に適用される目標の形にする。
//
// updatedAt には作成日時を入れる。同じ日の上書きでは変わらない
func manualTargetsOf(e openapi.ManualTargetEntry) openapi.ManualTargets {
	kcal := e.Kcal
	startsOn := openapi_types.Date{Time: e.StartsOn.Time}
	createdAt := e.CreatedAt

	return openapi.ManualTargets{
		ProteinG:  e.ProteinG,
		FatG:      e.FatG,
		CarbG:     e.CarbG,
		Kcal:      &kcal,
		StartsOn:  &startsOn,
		UpdatedAt: &createdAt,
	}
}
