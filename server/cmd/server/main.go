package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"codedock/internal/logger"
)

const (
	defaultHTTPAddress = ":8080"
	shutdownTimeout    = 10 * time.Second
)

func main() {
	logger.Init()
	log := logger.NewLogger("server")

	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = defaultHTTPAddress
	}

	server := &http.Server{
		Addr:              address,
		Handler:           newRouter(log),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", address)
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
