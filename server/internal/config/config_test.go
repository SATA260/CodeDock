package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
	t.Setenv("LLM_THINKING", "")
	t.Setenv("LLM_MAX_INPUT_TOKENS", "")
	t.Setenv("LLM_MAX_OUTPUT_TOKENS", "")
	t.Setenv("LLM_MAX_TURNS", "")
	t.Setenv("LLM_MAX_TOOL_CALLS", "")
	t.Setenv("LLM_MAX_WALL_TIME", "")
	t.Setenv("GIT_REPO", "")
	t.Setenv("PLUGIN_DIR", "")
	t.Setenv("PLUGIN_RPC_TIMEOUT", "")
	t.Setenv("CODEX_BIN", "")

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
	wantSuffix := filepath.Join("data", "codedock.db")
	if !strings.HasPrefix(cfg.DBDSN, "file:") || !strings.HasSuffix(cfg.DBDSN, wantSuffix) {
		t.Fatalf("DBDSN = %q, want file:.../%s", cfg.DBDSN, wantSuffix)
	}
	if cfg.LLMProvider != "fake" {
		t.Fatalf("LLMProvider = %q, want fake", cfg.LLMProvider)
	}
	if cfg.LLMModel != "fake" {
		t.Fatalf("LLMModel = %q, want fake", cfg.LLMModel)
	}
	if cfg.LLMThinking != "" || cfg.LLMMaxInputTokens != 256000 || cfg.LLMMaxOutputTokens != 65536 || cfg.LLMMaxTurns != 32 || cfg.LLMMaxToolCalls != 64 || cfg.LLMMaxWallTime != 20*time.Minute {
		t.Fatalf("llm limits = %+v", cfg)
	}
	if cfg.GitRepo != "" {
		t.Fatalf("GitRepo = %q, want empty", cfg.GitRepo)
	}
	if cfg.LLMConcurrency != 4 {
		t.Fatalf("LLMConcurrency = %d, want 4", cfg.LLMConcurrency)
	}
	if cfg.ToolConcurrency != 8 {
		t.Fatalf("ToolConcurrency = %d, want 8", cfg.ToolConcurrency)
	}
	if cfg.PluginDir != "" {
		t.Fatalf("PluginDir = %q, want empty", cfg.PluginDir)
	}
	if cfg.PluginRPCTimeout != 10*time.Second {
		t.Fatalf("PluginRPCTimeout = %s, want 10s", cfg.PluginRPCTimeout)
	}
	if cfg.CodexBin != "codex" {
		t.Fatalf("CodexBin = %q, want codex", cfg.CodexBin)
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
	t.Setenv("LLM_THINKING", "disabled")
	t.Setenv("LLM_MAX_INPUT_TOKENS", "1000")
	t.Setenv("LLM_MAX_OUTPUT_TOKENS", "2000")
	t.Setenv("LLM_MAX_TURNS", "3")
	t.Setenv("LLM_MAX_TOOL_CALLS", "4")
	t.Setenv("LLM_MAX_WALL_TIME", "1m")
	t.Setenv("GIT_REPO", "/tmp/repo")
	t.Setenv("PLUGIN_DIR", "/tmp/plugins")
	t.Setenv("PLUGIN_RPC_TIMEOUT", "2s")
	t.Setenv("CODEX_BIN", "/usr/local/bin/codex")

	cfg := Load()
	if cfg.HTTPAddr != ":9090" || cfg.LogLevel != "info" || cfg.DBEngine != "postgres" || cfg.DBDSN != "postgres://localhost" {
		t.Fatalf("Load() = %+v", cfg)
	}
	if cfg.LLMProvider != "openai" || cfg.LLMModel != "gpt-4o" || cfg.LLMAPIKey != "sk-test" || cfg.LLMBaseURL != "https://api.example.com/v1" {
		t.Fatalf("Load() LLM = %+v", cfg)
	}
	if cfg.LLMThinking != "disabled" || cfg.LLMMaxInputTokens != 1000 || cfg.LLMMaxOutputTokens != 2000 || cfg.LLMMaxTurns != 3 || cfg.LLMMaxToolCalls != 4 || cfg.LLMMaxWallTime != time.Minute {
		t.Fatalf("llm limits = %+v", cfg)
	}
	if cfg.GitRepo != "/tmp/repo" {
		t.Fatalf("GitRepo = %q, want /tmp/repo", cfg.GitRepo)
	}
	if cfg.PluginDir != "/tmp/plugins" || cfg.PluginRPCTimeout != 2*time.Second {
		t.Fatalf("plugin cfg = %+v", cfg)
	}
	if cfg.CodexBin != "/usr/local/bin/codex" {
		t.Fatalf("CodexBin = %q, want /usr/local/bin/codex", cfg.CodexBin)
	}
}

func TestDataDir(t *testing.T) {
	got := DataDir()
	if !strings.HasSuffix(got, string(filepath.Separator)+"data") && !strings.HasSuffix(got, "/data") {
		t.Fatalf("DataDir() = %q, want .../data", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(got), "pnpm-workspace.yaml")); err != nil {
		t.Fatalf("DataDir parent is not repo root: %v", err)
	}
}

func TestDefaultRoot(t *testing.T) {
	dir := t.TempDir()
	got := Config{GitRepo: dir}.DefaultRoot()
	want, _ := filepath.Abs(dir)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if (Config{}).DefaultRoot() == "" {
		t.Fatal("empty git repo should fall back to cwd")
	}
}

// TestParseDotEnvFile 校验注释、引号、export 与行尾注释。
func TestParseDotEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	body := "# comment\n\nexport LLM_PROVIDER=openai\nLLM_MODEL=\"gpt-4o\"\nLLM_API_KEY='sk-test'\nHTTP_ADDR=:9090 # listen\nEMPTY=\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	pairs, err := parseDotEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if pairs["LLM_PROVIDER"] != "openai" || pairs["LLM_MODEL"] != "gpt-4o" || pairs["LLM_API_KEY"] != "sk-test" {
		t.Fatalf("pairs = %#v", pairs)
	}
	if pairs["HTTP_ADDR"] != ":9090" || pairs["EMPTY"] != "" {
		t.Fatalf("pairs = %#v", pairs)
	}
}

// TestParseDotEnvLineRejectsInvalid 校验缺少等号时报错。
func TestParseDotEnvLineRejectsInvalid(t *testing.T) {
	if _, _, err := parseDotEnvLine("NOT_A_PAIR"); err == nil {
		t.Fatal("expected error")
	}
}
