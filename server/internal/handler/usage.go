package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	pkgagent "codedock/pkg/agent"
	"codedock/pkg/db/sqlite"
)

type UsageResponse struct {
	Records []pkgagent.UsageRecord `json:"records"`
}

// GetSessionUsage 按会话查询用量记录。
func (a *API) GetSessionUsage(w http.ResponseWriter, r *http.Request) {
	rows, err := a.q(r.Context()).ListUsageBySession(r.Context(), chi.URLParam(r, "session_id"))
	if err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	writeJSON(w, http.StatusOK, UsageResponse{Records: mapUsageRows(rows)})
}

// GetRunUsage 按 Run 查询用量记录。
func (a *API) GetRunUsage(w http.ResponseWriter, r *http.Request) {
	rows, err := a.q(r.Context()).ListUsageByRun(r.Context(), chi.URLParam(r, "run_id"))
	if err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	writeJSON(w, http.StatusOK, UsageResponse{Records: mapUsageRows(rows)})
}

// mapUsageRows 把用量行批量映射为领域对象。
func mapUsageRows(rows []sqlite.UsageRecord) []pkgagent.UsageRecord {
	out := make([]pkgagent.UsageRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapUsage(row))
	}
	return out
}
