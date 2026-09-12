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
	env := map[string]string{"DATABASE_URL": "postgres://localhost:5432/physique"}
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
		"DATABASE_URL": "postgres://localhost:5432/physique",
		"PORT":         "3000",
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
		"DATABASE_URL": "postgres://localhost:5432/physique",
		"PORT":         "http",
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
