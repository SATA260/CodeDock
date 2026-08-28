package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"codedock/internal/agent"
	"codedock/internal/config"
	"codedock/internal/events"
	"codedock/internal/handler"
	"codedock/internal/logger"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/agent/tool"
	"codedock/pkg/db"
)

const shutdownTimeout = 10 * time.Second

// main 打开数据库、跑迁移、装配 Runtime/Handler 并监听 HTTP，关闭时优雅退出。
func main() {
	cfg := config.Load()
	logger.Init(cfg.LogLevel)
	log := logger.NewLogger("server")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := db.Open(ctx, db.Config{Engine: db.Engine(cfg.DBEngine), DSN: cfg.DBDSN})
	if err != nil {
		log.Error("open database", "error", err)
		os.Exit(1)
	}
	defer client.Close()
	if err := db.Migrate(ctx, client.DB()); err != nil {
		log.Error("migrate database", "error", err)
		os.Exit(1)
	}

	queries := db.SQLiteQueries(client)
	bus := events.New()
	registry := tool.NewRegistry()
	_ = registry.Register(tool.Ping())
	runtime := agent.New(client, queries, bus, registry)
	runtime.Start(ctx)

	defaults := pkgagent.DefaultRunConfig(pkgagent.ModeAskForApproval, pkgagent.ModelConfig{
		Provider: cfg.LLMProvider,
		Model:    cfg.LLMModel,
		Options:  modelOptions(cfg),
	})
	api := handler.New(client, queries, runtime, bus, defaults)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           newRouter(log, api),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("api server stopped", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			log.Error("api server shutdown failed", "error", err)
		}
	}
}

// modelOptions 把 LLM API Key 与 BaseURL 编进 ModelConfig.Options。
func modelOptions(cfg config.Config) json.RawMessage {
	body, err := json.Marshal(map[string]string{
		"api_key":  cfg.LLMAPIKey,
		"base_url": cfg.LLMBaseURL,
	})
	if err != nil {
		return json.RawMessage("{}")
	}
	return body
}
