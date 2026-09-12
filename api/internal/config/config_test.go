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
