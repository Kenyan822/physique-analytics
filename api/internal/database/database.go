// Package database は PostgreSQL への接続を扱う。
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config は接続設定。
type Config struct {
	// DSN は接続文字列。Supabase では Supavisor（transaction mode, port 6543）を指す。
	// Cloud Run はインスタンスが増減するため、Postgres に直接つなぐと接続数を使い切る
	// （docs/05-インフラ設計 §2.2）。
	DSN string

	// MaxConns はこのインスタンスが張る上限。Supavisor の上限をインスタンス数で割った値にする。
	MaxConns int32
}

// Pool は pgxpool を包んで、アプリ側が必要とする操作だけを見せる。
type Pool struct {
	pool *pgxpool.Pool
}

// Open は接続プールを作り、疎通を確認してから返す。
func Open(ctx context.Context, cfg Config) (*Pool, error) {
	pc, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("DSN を解釈できない: %w", err)
	}
	if cfg.MaxConns > 0 {
		pc.MaxConns = cfg.MaxConns
	}
	// Supavisor の transaction mode ではサーバ側のプリペアドステートメントが
	// 接続をまたいで再利用できない。pgx の既定（キャッシュ）のままだと
	// "prepared statement already exists" で落ちる
	pc.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("接続プールを作れない: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("DB に疎通できない: %w", err)
	}

	return &Pool{pool: pool}, nil
}

// Ping は疎通を確認する。handler.Pinger を満たす。
func (p *Pool) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

// DB は repository に渡す接続。repository.DBTX を満たす。
func (p *Pool) DB() *pgxpool.Pool { return p.pool }

// Close は接続プールを閉じる。
func (p *Pool) Close() { p.pool.Close() }
