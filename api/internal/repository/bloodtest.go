package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// BloodTest は血液検査へのアクセス（要件 B-08）。
//
// **検査項目を固定しない。** クリニックやパネルによって項目が違うので、
// 検査票の表をそのまま写せる形で持つ。
type BloodTest struct {
	db DBTX
}

// NewBloodTest は BloodTest を作る。
func NewBloodTest(db DBTX) *BloodTest {
	return &BloodTest{db: db}
}

const bloodTestColumns = `id, date, clinic, note, created_at, updated_at, deleted_at`

const bloodTestItemColumns = `name, value, text_value, unit, ref_low, ref_high`

// List は血液検査を新しい順に返す。
func (r *BloodTest) List(ctx context.Context) ([]openapi.BloodTest, error) {
	const q = `select ` + bloodTestColumns + ` from blood_tests
		where deleted_at is null order by date desc`

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("血液検査の一覧を引けない: %w", err)
	}
	defer rows.Close()

	out := make([]openapi.BloodTest, 0, 8)
	for rows.Next() {
		t, err := scanBloodTest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("血液検査の一覧を読めない: %w", err)
	}

	for i := range out {
		if out[i].Items, err = r.listItems(ctx, out[i].Id); err != nil {
			return nil, err
		}
		n := countOutOfRange(out[i].Items)
		out[i].OutOfRangeCount = &n
	}

	return out, nil
}

// Get は血液検査を1件返す。
func (r *BloodTest) Get(ctx context.Context, id uuid.UUID) (openapi.BloodTest, error) {
	const q = `select ` + bloodTestColumns + ` from blood_tests
		where id = $1 and deleted_at is null`

	t, err := scanBloodTest(r.db.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.BloodTest{}, fmt.Errorf("血液検査 %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return openapi.BloodTest{}, fmt.Errorf("血液検査を取得できない: %w", err)
	}

	if t.Items, err = r.listItems(ctx, id); err != nil {
		return openapi.BloodTest{}, err
	}
	n := countOutOfRange(t.Items)
	t.OutOfRangeCount = &n

	return t, nil
}

// Create は血液検査を登録する。同じ日があれば ErrConflict。
func (r *BloodTest) Create(ctx context.Context, in openapi.BloodTestInput) (openapi.BloodTest, error) {
	const q = `insert into blood_tests (date, clinic, note) values ($1, $2, $3)
		returning ` + bloodTestColumns

	t, err := scanBloodTest(r.db.QueryRow(ctx, q, in.Date.Time, in.Clinic, in.Note))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return openapi.BloodTest{}, fmt.Errorf("血液検査 %s: %w",
			in.Date.Format("2006-01-02"), ErrConflict)
	}
	if err != nil {
		return openapi.BloodTest{}, fmt.Errorf("血液検査を登録できない: %w", err)
	}

	if err := r.replaceItems(ctx, t.Id, in.Items); err != nil {
		return openapi.BloodTest{}, err
	}

	return r.Get(ctx, t.Id)
}

// Update は血液検査を更新する。項目は全入れ替え。
func (r *BloodTest) Update(ctx context.Context, id uuid.UUID, in openapi.BloodTestInput) (openapi.BloodTest, error) {
	const q = `update blood_tests set date = $2, clinic = $3, note = $4, updated_at = $5
		where id = $1 and deleted_at is null
		returning ` + bloodTestColumns

	_, err := scanBloodTest(r.db.QueryRow(ctx, q, id, in.Date.Time, in.Clinic, in.Note, timeutil.Now()))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.BloodTest{}, fmt.Errorf("血液検査 %s: %w", id, ErrNotFound)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return openapi.BloodTest{}, fmt.Errorf("血液検査 %s: %w",
			in.Date.Format("2006-01-02"), ErrConflict)
	}
	if err != nil {
		return openapi.BloodTest{}, fmt.Errorf("血液検査を更新できない: %w", err)
	}

	if err := r.replaceItems(ctx, id, in.Items); err != nil {
		return openapi.BloodTest{}, err
	}

	return r.Get(ctx, id)
}

// SoftDelete は血液検査を論理削除する（ADR-0014）。
func (r *BloodTest) SoftDelete(ctx context.Context, id uuid.UUID) error {
	const q = `update blood_tests set deleted_at = $2, updated_at = $2
		where id = $1 and deleted_at is null`

	tag, err := r.db.Exec(ctx, q, id, timeutil.Now())
	if err != nil {
		return fmt.Errorf("血液検査を削除できない: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("血液検査 %s: %w", id, ErrNotFound)
	}

	return nil
}

func (r *BloodTest) listItems(ctx context.Context, id uuid.UUID) ([]openapi.BloodTestItem, error) {
	// 検査票の並び順で返す。名前で並べ替えると検査票と見比べられない
	const q = `select ` + bloodTestItemColumns + ` from blood_test_items
		where blood_test_id = $1 order by item_order`

	rows, err := r.db.Query(ctx, q, id)
	if err != nil {
		return nil, fmt.Errorf("検査項目を引けない: %w", err)
	}
	defer rows.Close()

	out := make([]openapi.BloodTestItem, 0, 32)
	for rows.Next() {
		var it openapi.BloodTestItem
		if err := rows.Scan(&it.Name, &it.Value, &it.TextValue, &it.Unit,
			&it.RefLow, &it.RefHigh); err != nil {
			return nil, fmt.Errorf("検査項目を読めない: %w", err)
		}
		// 判定はサーバが付ける。クライアントごとに基準の解釈が割れないように
		flag := openapi.RefFlag(analytics.JudgeRef(
			f64(it.Value), f64(it.RefLow), f64(it.RefHigh)))
		it.Flag = &flag
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("検査項目を読めない: %w", err)
	}

	return out, nil
}

func (r *BloodTest) replaceItems(ctx context.Context, id uuid.UUID, items []openapi.BloodTestItem) error {
	if _, err := r.db.Exec(ctx, `delete from blood_test_items where blood_test_id = $1`, id); err != nil {
		return fmt.Errorf("検査項目を消せない: %w", err)
	}

	const q = `
		insert into blood_test_items
			(blood_test_id, item_order, name, value, text_value, unit, ref_low, ref_high)
		values ($1, $2, $3, $4, $5, $6, $7, $8)`
	for i, it := range items {
		if _, err := r.db.Exec(ctx, q, id, i+1, it.Name, it.Value, it.TextValue,
			it.Unit, it.RefLow, it.RefHigh); err != nil {
			return fmt.Errorf("検査項目 %q を保存できない: %w", it.Name, err)
		}
	}

	return nil
}

func countOutOfRange(items []openapi.BloodTestItem) int {
	n := 0
	for _, it := range items {
		if it.Flag != nil && analytics.RefFlag(*it.Flag).OutOfRange() {
			n++
		}
	}

	return n
}

func scanBloodTest(row pgx.Row) (openapi.BloodTest, error) {
	var t openapi.BloodTest
	if err := row.Scan(&t.Id, &t.Date.Time, &t.Clinic, &t.Note,
		&t.CreatedAt, &t.UpdatedAt, &t.DeletedAt); err != nil {
		return openapi.BloodTest{}, err
	}
	t.Items = []openapi.BloodTestItem{}

	return t, nil
}

// f64 は *float32 を *float64 にする。生成される型は float32 だが、
// analytics は float64 で扱う。
func f64(v *float32) *float64 {
	if v == nil {
		return nil
	}
	f := float64(*v)

	return &f
}
