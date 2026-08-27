package main

import (
	"log/slog"
	"net/http"

	"codedock/internal/handler"
	"codedock/internal/logger"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func newRouter(log *slog.Logger) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(logger.Middleware(log))
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)

	router.Get("/health", handler.Health)

	return router
}
