package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	defaultHTTPAddress = ":8080"
	shutdownTimeout    = 10 * time.Second
)

func main() {
	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = defaultHTTPAddress
	}

	server := &http.Server{
		Addr:              address,
		Handler:           newRouter(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("API listening on %s", address)
		serverErrors <- server.ListenAndServe()
	}()

	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("API server stopped: %v", err)
		}
	case <-signalContext.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownContext); err != nil {
			log.Printf("API server shutdown failed: %v", err)
		}
	}
}
