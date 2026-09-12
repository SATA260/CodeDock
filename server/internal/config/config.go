package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config 是进程启动时一次性读取的环境配置。
type Config struct {
	HTTPAddr        string // HTTP 监听地址
	LogLevel        string
	DBEngine        string
	DBDSN           string
	LLMProvider     string
	LLMModel        string
	LLMAPIKey       string
	LLMBaseURL      string
	GitRepo         string // 默认仓库根；会话未指定工作目录时回落到这里，再否则 cwd
	LLMConcurrency  int    // 进程内同时进行的模型调用上限；0 表示不限制
	ToolConcurrency int    // 进程内同时执行的工具调用上限；0 表示不限制
}

// Load 从环境变量读取配置，未设置时使用默认值。
func Load() Config {
	return Config{
		HTTPAddr:        env("HTTP_ADDR", ":8080"),
		LogLevel:        env("LOG_LEVEL", "debug"),
		DBEngine:        env("DB_ENGINE", "sqlite"),
		DBDSN:           env("DB_DSN", "file:codedock.db"),
		LLMProvider:     env("LLM_PROVIDER", "fake"),
		LLMModel:        env("LLM_MODEL", "fake"),
		LLMAPIKey:       env("LLM_API_KEY", ""),
		LLMBaseURL:      env("LLM_BASE_URL", ""),
		GitRepo:         env("GIT_REPO", ""),
		LLMConcurrency:  envInt("LLM_CONCURRENCY", 4),
		ToolConcurrency: envInt("TOOL_CONCURRENCY", 8),
	}
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
