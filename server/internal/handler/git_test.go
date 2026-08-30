package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"codedock/internal/config"
	"codedock/internal/handler"
	pkgagent "codedock/pkg/agent"
)

func TestGitSkeletonRoutes(t *testing.T) {
	t.Setenv("GIT_REPO", t.TempDir())
	api := handler.New(nil, nil, nil, nil, pkgagent.RunConfigSnapshot{}, config.Load(), nil)
	r := chi.NewRouter()
	r.Get("/git/status", api.GitStatus)
	r.Get("/git/branches", api.GitListBranches)
	r.Get("/git/undo", api.GitListUndo)
	r.Get("/git/conflict", api.GitGetConflict)

	for _, path := range []string{"/git/status", "/git/branches", "/git/undo", "/git/conflict"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
}
