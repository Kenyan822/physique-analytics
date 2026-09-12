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

	// SupabaseJWKSURL は JWT の検証に使う公開鍵の取得先。
	// https://<project-ref>.supabase.co/auth/v1/.well-known/jwks.json
	// シークレットではない（公開鍵なので）。
	SupabaseJWKSURL string

	// AuthDisabled は認証を無効にする。ローカル開発専用。
	//
	// **既定で無効にはしない。** JWKS の設定を忘れたときに黙って認証なしで
	// 起動すると、本番が開いたまま動き続ける。明示的に指定させる。
	AuthDisabled bool
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

	if v, ok := lookup("AUTH_DISABLED"); ok && v == "true" {
		cfg.AuthDisabled = true
	}

	if jwks, ok := lookup("SUPABASE_JWKS_URL"); ok && jwks != "" {
		cfg.SupabaseJWKSURL = jwks
	} else if !cfg.AuthDisabled {
		return Config{}, errors.New("SUPABASE_JWKS_URL が設定されていない（ローカル開発なら AUTH_DISABLED=true）")
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
