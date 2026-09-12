// Package config は環境変数から起動設定を読む。
package config

import (
	"errors"
	"fmt"
	"strconv"
)

// Config は API サーバの起動設定。
type Config struct {
	// Port は待ち受けポート。Cloud Run は PORT を注入してくるので固定値にはできない。
	Port int

	// DatabaseURL は Postgres の接続文字列。Supabase では Supavisor を指す（port 6543）。
	DatabaseURL string

	// DatabaseMaxConns は1インスタンスが張る接続数の上限。
	// Cloud Run のインスタンス数 × この値が Supavisor の上限を超えないようにする。
	DatabaseMaxConns int32
}

// LookupEnv は os.LookupEnv と同じ形。テストで差し替えるために引数で受ける。
type LookupEnv func(key string) (string, bool)

// Load は環境変数から設定を読む。必須の値が欠けていればエラーを返す。
func Load(lookup LookupEnv) (Config, error) {
	cfg := Config{
		Port:             8080,
		DatabaseMaxConns: 5,
	}

	dsn, ok := lookup("DATABASE_URL")
	if !ok || dsn == "" {
		return Config{}, errors.New("DATABASE_URL が設定されていない")
	}
	cfg.DatabaseURL = dsn

	if v, ok := lookup("PORT"); ok && v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("PORT が数値ではない: %q", v)
		}
		cfg.Port = port
	}

	if v, ok := lookup("DATABASE_MAX_CONNS"); ok && v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			return Config{}, fmt.Errorf("DATABASE_MAX_CONNS が数値ではない: %q", v)
		}
		cfg.DatabaseMaxConns = int32(n)
	}

	return cfg, nil
}

// LoadForMCP は MCP サーバ用の設定を読む。
//
// 認証の設定を要求しない。MCP は手元で stdio 越しに動き、DB を直接読むため
// （ADR-0010）。HTTP を待ち受けないので、認証を挟む対象が無い。
func LoadForMCP(lookup LookupEnv) (Config, error) {
	// MCP は手元の1プロセスだけ。接続は少なくてよい
	cfg := Config{DatabaseMaxConns: 2}

	dsn, ok := lookup("DATABASE_URL")
	if !ok || dsn == "" {
		return Config{}, errors.New("DATABASE_URL が設定されていない")
	}
	cfg.DatabaseURL = dsn

	return cfg, nil
}
