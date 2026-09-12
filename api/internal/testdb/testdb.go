// Package testdb は DB を使うテストの共通処理。
//
// 実際の Postgres に接続する。sqlmock のような偽物を使わないのは、
// 検証したいことの大半（部分インデックス、enum、論理削除の絞り込み）が
// SQL と DB の挙動そのものだから。偽物では通ってしまう。
package testdb

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
)

// defaultDSN はローカルの docker compose で立てた Postgres。
// compose.yaml に書いてある開発用の固定値で、本番の認証情報ではない。
// 本番・CI は TEST_DATABASE_URL で上書きする。
//
//nolint:gosec // G101: 開発用の固定 DSN。秘密情報ではない
const defaultDSN = "postgres://physique:dev@localhost:5432/physique?sslmode=disable"

var (
	poolOnce sync.Once
	pool     *pgxpool.Pool
	poolErr  error
)

// Begin はテスト用のトランザクションを開き、テスト終了時に Rollback する。
//
// **各テストを自分のトランザクションに閉じ込めるのが目的。** 共有の DB に対して
// t.Parallel() で件数を数えると、並行する別テストの挿入で壊れる。
// 未コミットの変更は他のトランザクションから見えないので、これで隔離できる。
//
// DB を立てていない手元では Skip する。go test ./... が赤くならないようにするため。
func Begin(t *testing.T) pgx.Tx {
	t.Helper()

	p := connect(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatalf("トランザクションを開始できない: %v", err)
	}
	t.Cleanup(func() {
		// コミットしないので、テスト中の変更はすべて巻き戻る
		if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("Rollback: %v", err)
		}
	})

	return tx
}

func connect(t *testing.T) *pgxpool.Pool {
	t.Helper()

	poolOnce.Do(func() {
		dsn := os.Getenv("TEST_DATABASE_URL")
		if dsn == "" {
			dsn = defaultDSN
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		p, err := pgxpool.New(ctx, dsn)
		if err != nil {
			poolErr = err
			return
		}
		if err := p.Ping(ctx); err != nil {
			p.Close()
			poolErr = err
			return
		}
		pool = p
	})

	if poolErr != nil {
		t.Skipf("テスト用 DB に接続できない（docker compose up -d で起動する）: %v", poolErr)
	}

	return pool
}

// InsertExercise はテスト用の種目を1件入れる。
// Begin のトランザクション内なので、後始末は Rollback に任せてよい。
func InsertExercise(t *testing.T, tx pgx.Tx, name string, mg openapi.MuscleGroup) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := tx.QueryRow(context.Background(),
		`insert into exercises (name, muscle_group) values ($1, $2) returning id`,
		name, string(mg),
	).Scan(&id)
	if err != nil {
		t.Fatalf("テスト用の種目を作れない: %v", err)
	}

	return id
}

// RandomUUID は存在しない ID を作る。
func RandomUUID() uuid.UUID { return uuid.New() }
