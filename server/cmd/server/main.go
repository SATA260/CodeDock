package main

import (
	"context"
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
)

const shutdownTimeout = 10 * time.Second

func main() {
	cfg := config.Load()
	logger.Init(cfg.LogLevel)
	log := logger.NewLogger("server")

	bus := events.New()
	runtime := agent.New(nil, bus)
	api := handler.New(nil, runtime, bus)

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

	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("api server stopped", "error", err)
			os.Exit(1)
		}
	case <-signalContext.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownContext); err != nil {
			log.Error("api server shutdown failed", "error", err)
		}
	}
}
