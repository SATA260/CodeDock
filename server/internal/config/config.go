package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config 是进程启动时一次性读取的环境配置。
type Config struct {
	HTTPAddr         string // HTTP 监听地址
	LogLevel         string
	DBEngine         string
	DBDSN            string
	LLMProvider      string
	LLMModel         string
	LLMAPIKey        string
	LLMBaseURL       string
	GitRepo          string        // 默认仓库根；会话未指定工作目录时回落到这里，再否则 cwd
	LLMConcurrency   int           // 进程内同时进行的模型调用上限；0 表示不限制
	ToolConcurrency  int           // 进程内同时执行的工具调用上限；0 表示不限制
	PluginDir        string        // 插件目录；空则不拉进程
	PluginRPCTimeout time.Duration // 单次插件 RPC 超时
}

// Load 从环境变量读取配置，未设置时使用默认值。
func Load() Config {
	return Config{
		HTTPAddr:         env("HTTP_ADDR", ":8080"),
		LogLevel:         env("LOG_LEVEL", "debug"),
		DBEngine:         env("DB_ENGINE", "sqlite"),
		DBDSN:            env("DB_DSN", defaultSQLiteDSN()),
		LLMProvider:      env("LLM_PROVIDER", "fake"),
		LLMModel:         env("LLM_MODEL", "fake"),
		LLMAPIKey:        env("LLM_API_KEY", ""),
		LLMBaseURL:       env("LLM_BASE_URL", ""),
		GitRepo:          env("GIT_REPO", ""),
		LLMConcurrency:   envInt("LLM_CONCURRENCY", 4),
		ToolConcurrency:  envInt("TOOL_CONCURRENCY", 8),
		PluginDir:        env("PLUGIN_DIR", ""),
		PluginRPCTimeout: envDuration("PLUGIN_RPC_TIMEOUT", 10*time.Second),
	}
}

// defaultSQLiteDSN 默认库文件在仓根 data/，不写进 server/。
func defaultSQLiteDSN() string {
	return "file:" + filepath.Join(DataDir(), "codedock.db")
}

// DataDir 运行时文件目录：仓根下的 data/。找不到仓根则用 cwd/data。
func DataDir() string {
	return filepath.Join(repoRoot(), "data")
}

func repoRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	dir := cwd
	for i := 0; i <= 4; i++ {
		if isRepoRoot(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return cwd
}

func isRepoRoot(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "pnpm-workspace.yaml")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, "server", "go.mod"))
	return err == nil
}

// DefaultRoot 进程默认工作目录：GIT_REPO 的绝对路径，未设则 cwd。
func (c Config) DefaultRoot() string {
	repo := strings.TrimSpace(c.GitRepo)
	if repo != "" {
		if abs, err := filepath.Abs(repo); err == nil {
			return abs
		}
		return repo
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// env 读取环境变量，未设置时返回 fallback。
func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return d
}
