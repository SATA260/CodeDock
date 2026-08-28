package config

import "testing"

// TestLoadDefaults 校验未设置环境变量时的默认配置。
func TestLoadDefaults(t *testing.T) {
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("DB_ENGINE", "")
	t.Setenv("DB_DSN", "")
	t.Setenv("LLM_PROVIDER", "")
	t.Setenv("LLM_MODEL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_BASE_URL", "")

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
	if cfg.LLMProvider != "fake" {
		t.Fatalf("LLMProvider = %q, want fake", cfg.LLMProvider)
	}
	if cfg.LLMModel != "fake" {
		t.Fatalf("LLMModel = %q, want fake", cfg.LLMModel)
	}
}

// TestLoadFromEnv 校验环境变量覆盖默认配置。
func TestLoadFromEnv(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("DB_ENGINE", "postgres")
	t.Setenv("DB_DSN", "postgres://localhost")
	t.Setenv("LLM_PROVIDER", "openai")
	t.Setenv("LLM_MODEL", "gpt-4o")
	t.Setenv("LLM_API_KEY", "sk-test")
	t.Setenv("LLM_BASE_URL", "https://api.example.com/v1")

	cfg := Load()
	if cfg.HTTPAddr != ":9090" || cfg.LogLevel != "info" || cfg.DBEngine != "postgres" || cfg.DBDSN != "postgres://localhost" {
		t.Fatalf("Load() = %+v", cfg)
	}
	if cfg.LLMProvider != "openai" || cfg.LLMModel != "gpt-4o" || cfg.LLMAPIKey != "sk-test" || cfg.LLMBaseURL != "https://api.example.com/v1" {
		t.Fatalf("Load() LLM = %+v", cfg)
	}
}
