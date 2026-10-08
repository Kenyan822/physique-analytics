package repository

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// Postgres のエラーコード（SQLSTATE）。
const (
	checkViolation      = "23514"
	foreignKeyViolation = "23503"
	// uniqueViolation は repository.go にある（既存の ErrConflict への変換が使う）
)

// pgErrorOf は err の連鎖から *pgconn.PgError を取り出す。
//
// **文字列で判別しない。** メッセージは日本語環境や将来の Postgres で変わりうる。
// コードは仕様で固定されている。
func pgErrorOf(err error) *pgconn.PgError {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr
	}

	return nil
}

func hasCode(err error, code string) bool {
	pgErr := pgErrorOf(err)

	return pgErr != nil && pgErr.Code == code
}

// IsCheckViolation は err が CHECK 制約違反（23514）かを判定する。
//
// 送った値が許されていない。**サーバの内部エラーではない**ので、ハンドラは 422 にする。
func IsCheckViolation(err error) bool { return hasCode(err, checkViolation) }

// IsForeignKeyViolation は err が外部キー違反（23503）かを判定する。
//
// 参照先が無い（存在しない id を送った）。ハンドラは 422 にする。
func IsForeignKeyViolation(err error) bool { return hasCode(err, foreignKeyViolation) }

// IsUniqueViolation は err が一意制約違反（23505）かを判定する。
//
// 個別に ErrConflict へ変換していない経路の保険。ハンドラは 409 にする。
func IsUniqueViolation(err error) bool { return hasCode(err, uniqueViolation) }

// ConstraintName は制約違反の制約名を返す。制約違反でなければ空。
func ConstraintName(err error) string {
	if pgErr := pgErrorOf(err); pgErr != nil {
		return pgErr.ConstraintName
	}

	return ""
}
