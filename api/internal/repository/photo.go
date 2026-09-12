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

// Photo は身体写真のメタデータへのアクセス（要件 B-04 / B-05 / B-07）。
//
// **画像そのものは持たない。** R2 に置き（ADR-0008）、ここには位置と
// メタデータだけを持つ。
type Photo struct {
	db DBTX
}

// NewPhoto は Photo を作る。
func NewPhoto(db DBTX) *Photo {
	return &Photo{db: db}
}

// PhotoInput は写真の登録の入力。
type PhotoInput struct {
	Date       openapi_types.Date
	Pose       openapi.PhotoPose
	StorageKey string
	MimeType   string
	ByteSize   int64
	Note       *string
}

const photoColumns = `id, date, pose, storage_key, mime_type, byte_size, note,
	created_at, updated_at, deleted_at`

// PhotoRow は DB の行。StorageKey は API には出さない（署名付きURLだけ返す）。
type PhotoRow struct {
	Photo      openapi.BodyPhoto
	StorageKey string
}

// List は期間内の写真を新しい順に返す。
func (r *Photo) List(ctx context.Context, from, to *openapi_types.Date) ([]PhotoRow, error) {
	const q = `
		select ` + photoColumns + `
		from body_photos
		where deleted_at is null
		  and ($1::date is null or date >= $1::date)
		  and ($2::date is null or date <= $2::date)
		order by date desc, pose`

	return r.query(ctx, q, dateOrNil(from), dateOrNil(to))
}

// Guide は向きごとに、指定日より前の直近の1枚を返す（要件 B-05）。
//
// **撮影時に前回写真を重ねるため。** 同じ距離・角度・ポーズを再現しないと
// 比較が成立しない（docs/02-データモデル.md）。
func (r *Photo) Guide(ctx context.Context, before openapi_types.Date) ([]PhotoRow, error) {
	const q = `
		select distinct on (pose) ` + photoColumns + `
		from body_photos
		where deleted_at is null and date < $1::date
		order by pose, date desc`

	return r.query(ctx, q, before.Time)
}

// Create は写真のメタデータを登録する。同じ日・同じ向きがあれば ErrConflict。
func (r *Photo) Create(ctx context.Context, in PhotoInput) (PhotoRow, error) {
	const q = `
		insert into body_photos (date, pose, storage_key, mime_type, byte_size, note)
		values ($1, $2, $3, $4, $5, $6)
		returning ` + photoColumns

	row, err := scanPhoto(r.db.QueryRow(ctx, q,
		in.Date.Time, string(in.Pose), in.StorageKey, in.MimeType, in.ByteSize, in.Note))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return PhotoRow{}, fmt.Errorf("%s の %s: %w",
			in.Date.Format("2006-01-02"), in.Pose, ErrConflict)
	}
	if err != nil {
		return PhotoRow{}, fmt.Errorf("写真を登録できない: %w", err)
	}

	return row, nil
}

// SoftDelete は写真を論理削除する（ADR-0014）。
//
// **R2 のオブジェクトは消さない。** 誤操作からの復旧を優先する。
// 容量は10GBあり、写真は月3枚なので圧迫しない。
func (r *Photo) SoftDelete(ctx context.Context, id uuid.UUID) error {
	const q = `update body_photos set deleted_at = $2, updated_at = $2
		where id = $1 and deleted_at is null`

	tag, err := r.db.Exec(ctx, q, id, timeutil.Now())
	if err != nil {
		return fmt.Errorf("写真を削除できない: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("写真 %s: %w", id, ErrNotFound)
	}

	return nil
}

func (r *Photo) query(ctx context.Context, q string, args ...any) ([]PhotoRow, error) {
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("写真を引けない: %w", err)
	}
	defer rows.Close()

	out := make([]PhotoRow, 0, 16)
	for rows.Next() {
		p, err := scanPhoto(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("写真を読めない: %w", err)
	}

	return out, nil
}

func scanPhoto(row pgx.Row) (PhotoRow, error) {
	var p openapi.BodyPhoto
	var pose, key string
	err := row.Scan(&p.Id, &p.Date.Time, &pose, &key, &p.MimeType, &p.ByteSize, &p.Note,
		&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt)
	if err != nil {
		return PhotoRow{}, err
	}
	p.Pose = openapi.PhotoPose(pose)

	return PhotoRow{Photo: p, StorageKey: key}, nil
}
