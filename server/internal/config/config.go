package config

import "os"

// Config 是进程启动时一次性读取的环境配置。
type Config struct {
	HTTPAddr string
	LogLevel string
	DBEngine string
	DBDSN    string
}

// Load 从环境变量读取配置，未设置时使用默认值。
func Load() Config {
	return Config{
		HTTPAddr: env("HTTP_ADDR", ":8080"),
		LogLevel: env("LOG_LEVEL", "debug"),
		DBEngine: env("DB_ENGINE", "sqlite"),
		DBDSN:    env("DB_DSN", "file:codedock.db"),
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
