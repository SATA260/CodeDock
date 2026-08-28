package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	pkgagent "codedock/pkg/agent"
	"codedock/pkg/db/sqlite"
)

type UsageResponse struct {
	Records []pkgagent.UsageRecord `json:"records"`
	PageInfo
}

// GetSessionUsage 按会话分页查询用量记录。
func (a *API) GetSessionUsage(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePageQuery(r, usagePageDefaults)
	if err != nil {
		writeError(w, err)
		return
	}
	q := a.q(r.Context())
	sessionID := chi.URLParam(r, "session_id")
	total, err := q.CountUsageBySession(r.Context(), sessionID)
	if err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	rows, err := q.ListUsageBySession(r.Context(), sqlite.ListUsageBySessionParams{
		SessionID: sessionID,
		SortBy:    page.SortBy,
		SortOrder: page.SortOrder,
		Limit:     int64(page.Limit()),
		Offset:    int64(page.Offset()),
	})
	if err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	writeJSON(w, http.StatusOK, UsageResponse{Records: mapUsageRows(rows), PageInfo: page.Info(total)})
}

// GetRunUsage 按 Run 分页查询用量记录。
func (a *API) GetRunUsage(w http.ResponseWriter, r *http.Request) {
	page, err := ParsePageQuery(r, usagePageDefaults)
	if err != nil {
		writeError(w, err)
		return
	}
	q := a.q(r.Context())
	runID := chi.URLParam(r, "run_id")
	total, err := q.CountUsageByRun(r.Context(), runID)
	if err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	rows, err := q.ListUsageByRun(r.Context(), sqlite.ListUsageByRunParams{
		RunID:     runID,
		SortBy:    page.SortBy,
		SortOrder: page.SortOrder,
		Limit:     int64(page.Limit()),
		Offset:    int64(page.Offset()),
	})
	if err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	writeJSON(w, http.StatusOK, UsageResponse{Records: mapUsageRows(rows), PageInfo: page.Info(total)})
}

// mapUsageRows 把用量行批量映射为领域对象。
func mapUsageRows(rows []sqlite.UsageRecord) []pkgagent.UsageRecord {
	out := make([]pkgagent.UsageRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapUsage(row))
	}
	return out
}
