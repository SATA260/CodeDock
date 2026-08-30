package config

import "os"

// Config 是进程启动时一次性读取的环境配置。
type Config struct {
	HTTPAddr    string
	LogLevel    string
	DBEngine    string
	DBDSN       string
	LLMProvider string
	LLMModel    string
	LLMAPIKey   string
	LLMBaseURL  string
	GitRepo     string
}

// Load 从环境变量读取配置，未设置时使用默认值。
func Load() Config {
	return Config{
		HTTPAddr:    env("HTTP_ADDR", ":8080"),
		LogLevel:    env("LOG_LEVEL", "debug"),
		DBEngine:    env("DB_ENGINE", "sqlite"),
		DBDSN:       env("DB_DSN", "file:codedock.db"),
		LLMProvider: env("LLM_PROVIDER", "fake"),
		LLMModel:    env("LLM_MODEL", "fake"),
		LLMAPIKey:   env("LLM_API_KEY", ""),
		LLMBaseURL:  env("LLM_BASE_URL", ""),
		GitRepo:     env("GIT_REPO", ""),
	}
}

// env 读取环境变量，未设置时返回 fallback。
func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
