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
	HTTPAddr           string // HTTP 监听地址
	LogLevel           string
	DBEngine           string
	DBDSN              string
	LLMProvider        string
	LLMModel           string
	LLMAPIKey          string
	LLMBaseURL         string
	LLMThinking        string        // 思考开关：enabled | disabled；空则沿用模型默认
	LLMMaxInputTokens  int64         // 单次 Run 的输入 token 上限，含上下文
	LLMMaxOutputTokens int64         // 单次模型回复的输出 token 上限，思考和正文共用
	LLMMaxTurns        int           // 单次 Run 最多调用模型的轮数
	LLMMaxToolCalls    int           // 单次 Run 最多执行的工具次数
	LLMMaxWallTime     time.Duration // 单次 Run 的墙钟上限
	GitRepo            string        // 默认仓库根；会话未指定工作目录时回落到这里，再否则 cwd
	LLMConcurrency     int           // 进程内同时进行的模型调用上限；0 表示不限制
	ToolConcurrency    int           // 进程内同时执行的工具调用上限；0 表示不限制
	PluginDir          string        // 插件目录；空则不拉进程
	PluginRPCTimeout   time.Duration // 单次插件 RPC 超时
	CodexBin           string        // 本机 Codex CLI；未安装时主服务仍可启动
	EvaluatorProvider  string        // 独立复审模型供应商；空则回落主模型
	EvaluatorModel     string        // 独立复审模型名
	EvaluatorAPIKey    string        // 复审模型 Key；空则回落 LLM_API_KEY
	EvaluatorBaseURL   string        // 复审模型地址；空则回落 LLM_BASE_URL
	SubagentProvider   string        // explore 子代理供应商；空则回落复审模型
	SubagentModel      string        // explore 子代理模型名
	SubagentAPIKey     string        // 子代理 Key；空则回落复审/主模型
	SubagentBaseURL    string        // 子代理地址；空则回落复审/主模型
}

// Load 从环境变量读取配置，未设置时使用默认值。
func Load() Config {
	return Config{
		HTTPAddr:           env("HTTP_ADDR", ":8080"),
		LogLevel:           env("LOG_LEVEL", "debug"),
		DBEngine:           env("DB_ENGINE", "sqlite"),
		DBDSN:              env("DB_DSN", defaultSQLiteDSN()),
		LLMProvider:        env("LLM_PROVIDER", "fake"),
		LLMModel:           env("LLM_MODEL", "fake"),
		LLMAPIKey:          env("LLM_API_KEY", ""),
		LLMBaseURL:         env("LLM_BASE_URL", ""),
		LLMThinking:        env("LLM_THINKING", ""),
		LLMMaxInputTokens:  envInt64("LLM_MAX_INPUT_TOKENS", 256000),
		LLMMaxOutputTokens: envInt64("LLM_MAX_OUTPUT_TOKENS", 65536),
		LLMMaxTurns:        envInt("LLM_MAX_TURNS", 32),
		LLMMaxToolCalls:    envInt("LLM_MAX_TOOL_CALLS", 64),
		LLMMaxWallTime:     envDuration("LLM_MAX_WALL_TIME", 20*time.Minute),
		GitRepo:            env("GIT_REPO", ""),
		LLMConcurrency:     envInt("LLM_CONCURRENCY", 4),
		ToolConcurrency:    envInt("TOOL_CONCURRENCY", 8),
		PluginDir:          env("PLUGIN_DIR", ""),
		PluginRPCTimeout:   envDuration("PLUGIN_RPC_TIMEOUT", 10*time.Second),
		CodexBin:           env("CODEX_BIN", "codex"),
		EvaluatorProvider:  env("EVALUATOR_PROVIDER", ""),
		EvaluatorModel:     env("EVALUATOR_MODEL", ""),
		EvaluatorAPIKey:    env("EVALUATOR_API_KEY", ""),
		EvaluatorBaseURL:   env("EVALUATOR_BASE_URL", ""),
		SubagentProvider:   env("SUBAGENT_PROVIDER", ""),
		SubagentModel:      env("SUBAGENT_MODEL", ""),
		SubagentAPIKey:     env("SUBAGENT_API_KEY", ""),
		SubagentBaseURL:    env("SUBAGENT_BASE_URL", ""),
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

// repoRoot 从 cwd 向上找仓根（有 pnpm-workspace.yaml 或 AGENTS.md+server/go.mod）。
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

// isRepoRoot 判断 dir 是否是本仓根。
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

// envInt64 读 int64 环境变量，未设或解析失败时用 fallback。
func envInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

// envInt 读整数环境变量，未设或解析失败时用 fallback。
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

// envDuration 读 duration 环境变量，未设或解析失败时用 fallback。
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
