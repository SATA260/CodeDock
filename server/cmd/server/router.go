package main

import (
	"log/slog"
	"net/http"

	"codedock/internal/handler"
	"codedock/internal/logger"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func newRouter(log *slog.Logger, api *handler.API) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(logger.Middleware(log))
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)

	router.Get("/health", handler.Health)
	if api != nil {
		router.Route("/sessions", func(r chi.Router) {
			r.Post("/", api.CreateSession)
			r.Get("/", api.ListSessions)
			r.Get("/{session_id}", api.GetSession)
			r.Patch("/{session_id}", api.UpdateSession)
			r.Post("/{session_id}/archive", api.ArchiveSession)
			r.Post("/{session_id}/runs", api.StartRun)
			r.Post("/{session_id}/messages", api.CreateMessage)
			r.Get("/{session_id}/messages", api.ListMessages)
			r.Delete("/{session_id}/messages/{message_id}", api.DeleteMessage)
			r.Get("/{session_id}/events", api.SubscribeEvents)
			r.Get("/{session_id}/usage", api.GetSessionUsage)
			r.Get("/{session_id}/approvals", api.ListApprovals)
		})
		router.Route("/runs", func(r chi.Router) {
			r.Get("/{run_id}", api.GetRun)
			r.Get("/{run_id}/usage", api.GetRunUsage)
			r.Post("/{run_id}/continue", api.ContinueRun)
			r.Post("/{run_id}/retry", api.RetryRun)
			r.Post("/{run_id}/cancel", api.CancelRun)
		})
		router.Route("/approvals", func(r chi.Router) {
			r.Get("/{approval_id}", api.GetApproval)
			r.Post("/{approval_id}/decision", api.DecideApproval)
		})
		router.Route("/memories", func(r chi.Router) {
			r.Get("/", api.ListTextMemories)
			r.Get("/{scope}/{scope_id}", api.GetTextMemory)
			r.Delete("/{scope}/{scope_id}", api.DeleteTextMemory)
		})
	}

	return router
}
