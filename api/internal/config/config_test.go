package config_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/config"
)

func TestLoad_必須のDATABASE_URLがなければエラー(t *testing.T) {
	_, err := config.Load(func(string) (string, bool) { return "", false })
	if err == nil {
		t.Fatal("エラーを期待したが nil")
	}
}

func TestLoad_既定値(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":      "postgres://localhost:5432/physique",
		"SUPABASE_JWKS_URL": "https://example.supabase.co/auth/v1/.well-known/jwks.json",
	}
	cfg, err := config.Load(lookup(env))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Cloud Run は PORT を渡してくるが、ローカルでは未設定で動く必要がある
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.DatabaseMaxConns != 5 {
		t.Errorf("DatabaseMaxConns = %d, want 5", cfg.DatabaseMaxConns)
	}
}

func TestLoad_PORTを環境変数から読む(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":      "postgres://localhost:5432/physique",
		"SUPABASE_JWKS_URL": "https://example.supabase.co/auth/v1/.well-known/jwks.json",
		"PORT":              "3000",
	}
	cfg, err := config.Load(lookup(env))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 3000 {
		t.Errorf("Port = %d, want 3000", cfg.Port)
	}
}

func TestLoad_PORTが数値でなければエラー(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":      "postgres://localhost:5432/physique",
		"SUPABASE_JWKS_URL": "https://example.supabase.co/auth/v1/.well-known/jwks.json",
		"PORT":              "http",
	}
	if _, err := config.Load(lookup(env)); err == nil {
		t.Fatal("エラーを期待したが nil")
	}
}

func lookup(env map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := env[k]
		return v, ok
	}
}

// MCP は手元で stdio 越しに動き DB を直接読む。認証の設定は要らない
func TestLoadForMCP_DATABASE_URLだけで足りる(t *testing.T) {
	env := map[string]string{"DATABASE_URL": "postgres://localhost:5432/physique"}
	cfg, err := config.LoadForMCP(lookup(env))
	if err != nil {
		t.Fatalf("LoadForMCP: %v", err)
	}
	if cfg.DatabaseURL != env["DATABASE_URL"] {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.DatabaseMaxConns <= 0 {
		t.Errorf("DatabaseMaxConns = %d", cfg.DatabaseMaxConns)
	}
}

func TestLoadForMCP_DATABASE_URLが無ければエラー(t *testing.T) {
	if _, err := config.LoadForMCP(func(string) (string, bool) { return "", false }); err == nil {
		t.Fatal("エラーを期待したが nil")
	}
}

// 認証の設定を忘れたまま起動できてしまうと、本番が開いたまま動き続ける
func TestLoad_JWKSもAUTH_DISABLEDも無ければエラー(t *testing.T) {
	env := map[string]string{"DATABASE_URL": "postgres://localhost:5432/physique"}
	if _, err := config.Load(lookup(env)); err == nil {
		t.Fatal("エラーを期待したが nil")
	}
}

func TestLoad_AUTH_DISABLEDならJWKSは不要(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":  "postgres://localhost:5432/physique",
		"AUTH_DISABLED": "true",
	}
	cfg, err := config.Load(lookup(env))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.AuthDisabled {
		t.Error("AuthDisabled が false")
	}
}

// "1" や "yes" では無効にならない。取り違えて本番を開けないようにする
func TestLoad_AUTH_DISABLEDはtrueだけを受け付ける(t *testing.T) {
	for _, v := range []string{"1", "yes", "TRUE", "on"} {
		env := map[string]string{
			"DATABASE_URL":  "postgres://localhost:5432/physique",
			"AUTH_DISABLED": v,
		}
		if _, err := config.Load(lookup(env)); err == nil {
			t.Errorf("AUTH_DISABLED=%q で認証が無効になった", v)
		}
	}
}

func TestLoad_推定は任意(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(lookup(map[string]string{
		"DATABASE_URL":      "postgres://x",
		"SUPABASE_JWKS_URL": "https://x/jwks.json",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// **未設定でも起動する。** 推定だけが使えない状態になる
	if cfg.VisionAPIKey != "" {
		t.Errorf("VisionAPIKey = %q, want 空", cfg.VisionAPIKey)
	}
	// 既定モデルは要件のコスト試算に合わせる
	if cfg.VisionModel == "" {
		t.Error("VisionModel が空。既定を持たせる")
	}
}

func TestLoad_ALLOWED_USER_IDSを分ける(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":      "postgres://localhost:5432/physique",
		"SUPABASE_JWKS_URL": "https://example.supabase.co/auth/v1/.well-known/jwks.json",
		// 手で書くので、余分な空白と末尾のカンマは想定内
		"ALLOWED_USER_IDS": " 11111111-1111-1111-1111-111111111111 ,22222222-2222-2222-2222-222222222222, ",
	}
	cfg, err := config.Load(lookup(env))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
	}
	if len(cfg.AllowedUserIDs) != len(want) {
		t.Fatalf("AllowedUserIDs = %v, want %v", cfg.AllowedUserIDs, want)
	}
	for i, w := range want {
		if cfg.AllowedUserIDs[i] != w {
			t.Errorf("AllowedUserIDs[%d] = %q, want %q", i, cfg.AllowedUserIDs[i], w)
		}
	}
}

func TestLoad_ALLOWED_USER_IDSが未設定なら空(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":      "postgres://localhost:5432/physique",
		"SUPABASE_JWKS_URL": "https://example.supabase.co/auth/v1/.well-known/jwks.json",
	}
	cfg, err := config.Load(lookup(env))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// 未設定は「全員通す」。設定を足すまで挙動を変えない
	if len(cfg.AllowedUserIDs) != 0 {
		t.Errorf("AllowedUserIDs = %v, want 空", cfg.AllowedUserIDs)
	}
}
