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

	// VisionAPIKey は写真からの PFC 推定に使う API キー（要件 N-06）。
	//
	// **空なら推定を実行しない。** 未設定のまま動かしても課金は発生せず、
	// 推定のエンドポイントだけが 503 を返す。
	VisionAPIKey string

	// VisionModel は推定に使うモデル。要件のコスト試算は Haiku 相当
	// （年1,095回で約3ドル）。
	VisionModel string

	// VisionEndpoint は推定 API の宛先。空なら Anthropic の既定。
	//
	// **差し替えられるようにしてあるのは、課金を発生させずに経路全体を
	// 確かめるため。** 偽のサーバを立てて手元で通す。
	VisionEndpoint string

	// R2* は写真の置き場（要件 B-04、ADR-0008）。
	//
	// **未設定でも起動する。** 写真の機能だけが 503 を返す。
	R2AccountID       string
	R2Bucket          string
	R2AccessKeyID     string
	R2SecretAccessKey string

	// AuthDisabled は認証を無効にする。ローカル開発専用。
	//
	// **既定で無効にはしない。** JWKS の設定を忘れたときに黙って認証なしで
	// 起動すると、本番が開いたまま動き続ける。明示的に指定させる。
	AuthDisabled bool
}

// defaultVisionModel は推定の既定モデル。
//
// 要件のコスト試算（年1,095回・入力1,700 / 出力200トークン）が
// Haiku 相当で年3ドル程度という前提なので、既定はこれにする。
const defaultVisionModel = "claude-haiku-4-5-20251001"

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

	// 推定は任意。設定しなければ使えないだけで、起動は通す
	if v, ok := lookup("ANTHROPIC_API_KEY"); ok {
		cfg.VisionAPIKey = v
	}
	cfg.VisionModel = defaultVisionModel
	if v, ok := lookup("VISION_MODEL"); ok && v != "" {
		cfg.VisionModel = v
	}
	if v, ok := lookup("VISION_ENDPOINT"); ok {
		cfg.VisionEndpoint = v
	}

	// 写真も任意。設定しなければ使えないだけ
	cfg.R2AccountID, _ = lookup("R2_ACCOUNT_ID")
	cfg.R2Bucket, _ = lookup("R2_BUCKET")
	cfg.R2AccessKeyID, _ = lookup("R2_ACCESS_KEY_ID")
	cfg.R2SecretAccessKey, _ = lookup("R2_SECRET_ACCESS_KEY")

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
