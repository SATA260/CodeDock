package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"codedock/internal/agent"
	agenttools "codedock/internal/agent/tools"
	boardpkg "codedock/internal/board"
	intcodex "codedock/internal/codex"
	"codedock/internal/config"
	"codedock/internal/events"
	"codedock/internal/handler"
	codexhttp "codedock/internal/handler/codex"
	"codedock/internal/logger"
	"codedock/internal/pluginhost"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/db"
)

const shutdownTimeout = 10 * time.Second

// main 打开数据库、跑迁移、装配 Runtime/Handler 并监听 HTTP，关闭时优雅退出。
func main() {
	if err := config.LoadDotEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "load .env: %v\n", err)
		os.Exit(1)
	}
	cfg := config.Load()
	logger.Init(cfg.LogLevel)
	log := logger.NewLogger("server")
	log.Info("config loaded", "http_addr", cfg.HTTPAddr, "llm_provider", cfg.LLMProvider, "llm_model", cfg.LLMModel)

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
	model := pkgagent.ModelConfig{
		Provider: cfg.LLMProvider,
		Model:    cfg.LLMModel,
		Options:  modelOptions(cfg),
	}
	runtime := agent.New(client, queries, bus, nil, logger.NewLogger("agent"), agenttools.Ports{
		WorkspaceRoot: cfg.DefaultRoot(), // 进程回落；会话工作区由 Handler 创建时冻结
	})
	runtime.SetModel(model)
	runtime.SetConcurrency(cfg.LLMConcurrency, cfg.ToolConcurrency)
	log.Info("concurrency", "llm", cfg.LLMConcurrency, "tool", cfg.ToolConcurrency)

	var pluginHost *pluginhost.Host
	if cfg.PluginDir != "" {
		host, err := pluginhost.Load(ctx, pluginhost.Options{
			Dir:      cfg.PluginDir,
			Timeout:  cfg.PluginRPCTimeout,
			Registry: runtime.Tools(),
			Queries:  queries,
			Model:    model,
			Log:      logger.NewLogger("plugin"),
		})
		if err != nil {
			log.Error("load plugins", "error", err)
			os.Exit(1)
		}
		pluginHost = host
		if pluginHost != nil {
			runtime.SetDispatcher(pluginHost)
			pluginHost.Attach(bus)
			log.Info("plugins loaded", "dir", cfg.PluginDir)
		}
	}
	if pluginHost != nil {
		defer func() { _ = pluginHost.Close() }()
	}

	runtime.Start(ctx)

	defaults := pkgagent.DefaultRunConfig(pkgagent.WorkAgent, model)
	defaults.EvaluatorModel = sideModel(cfg.EvaluatorProvider, cfg.EvaluatorModel, cfg.EvaluatorAPIKey, cfg.EvaluatorBaseURL, model)
	defaults.SubagentModel = sideModel(cfg.SubagentProvider, cfg.SubagentModel, cfg.SubagentAPIKey, cfg.SubagentBaseURL, defaults.EvaluatorModel)
	api := handler.New(client, queries, runtime, bus, defaults, cfg, logger.NewLogger("handler"))
	codexRT := intcodex.New(intcodex.Options{Bin: cfg.CodexBin})
	defer func() { _ = codexRT.Close() }()
	api.SetCodex(codexRT)
	codexAPI := codexhttp.New(codexRT)
	codexAPI.SetPacket(func(ctx context.Context, sessionID string) string {
		pkt, err := boardpkg.BuildPacket(ctx, queries, boardpkg.EngineCodex, sessionID)
		if err != nil {
			return ""
		}
		return pkt.Text
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           newRouter(log, api, codexAPI),
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
	return encodeModelOptions(cfg.LLMAPIKey, cfg.LLMBaseURL)
}

// sideModel 组装复审或子代理模型；未配置时回落 fallback。
func sideModel(provider, name, apiKey, baseURL string, fallback pkgagent.ModelConfig) pkgagent.ModelConfig {
	out := fallback
	if strings.TrimSpace(provider) != "" {
		out.Provider = provider
	}
	if strings.TrimSpace(name) != "" {
		out.Model = name
	}
	if strings.TrimSpace(apiKey) != "" || strings.TrimSpace(baseURL) != "" {
		key := apiKey
		if key == "" {
			key = optionString(fallback.Options, "api_key")
		}
		url := baseURL
		if url == "" {
			url = optionString(fallback.Options, "base_url")
		}
		out.Options = encodeModelOptions(key, url)
	}
	return out
}

// encodeModelOptions 把 Key 与 BaseURL 编成模型 Options。
func encodeModelOptions(apiKey, baseURL string) json.RawMessage {
	body, err := json.Marshal(map[string]string{
		"api_key":  apiKey,
		"base_url": baseURL,
	})
	if err != nil {
		return json.RawMessage("{}")
	}
	return body
}

// optionString 从模型 Options 里读一个字符串字段。
func optionString(raw json.RawMessage, key string) string {
	var fields map[string]string
	if json.Unmarshal(raw, &fields) != nil {
		return ""
	}
	return fields[key]
}
