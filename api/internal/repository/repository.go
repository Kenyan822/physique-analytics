// Package repository は DB アクセスをまとめる。
//
// sqlc をまだ入れていないのは、今のところクエリが少なく、生成物を読む手間が
// 直接書くより大きいため（docs/06-技術選定.md）。クエリが増えたら切り替える。
package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrNotFound は対象が存在しない、または論理削除済みであることを表す。
//
// pgx.ErrNoRows をそのまま上に返さない。ハンドラ層が pgx を import する必要が
// なくなり、DB を差し替えたときに影響が repository 内で止まる。
var ErrNotFound = errors.New("見つからない")

// IsNotFound は err が ErrNotFound かを判定する。
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// DBTX は *pgxpool.Pool と pgx.Tx の両方が満たす。
//
// テストをトランザクション内で走らせて最後に Rollback するために挟んでいる。
// これがないと、並行するテスト同士が同じ行を見てしまい、件数のアサーションが
// 他のテストの挿入で壊れる。
type DBTX interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}
