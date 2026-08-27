package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("DB_ENGINE", "")
	t.Setenv("DB_DSN", "")

	cfg := Load()
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.DBEngine != "sqlite" {
		t.Fatalf("DBEngine = %q, want sqlite", cfg.DBEngine)
	}
	if cfg.DBDSN != "file:codedock.db" {
		t.Fatalf("DBDSN = %q, want file:codedock.db", cfg.DBDSN)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("DB_ENGINE", "postgres")
	t.Setenv("DB_DSN", "postgres://localhost")

	cfg := Load()
	if cfg.HTTPAddr != ":9090" || cfg.LogLevel != "info" || cfg.DBEngine != "postgres" || cfg.DBDSN != "postgres://localhost" {
		t.Fatalf("Load() = %+v", cfg)
	}
}
