package handler

import (
	"net/http"

	pkgagent "codedock/pkg/agent"
)

type UsageResponse struct {
	Records []pkgagent.UsageRecord
}

// GetSessionUsage 按会话查询用量记录。
// TODO: 按 session_id 查询用量记录。
func (a *API) GetSessionUsage(w http.ResponseWriter, _ *http.Request) {
	if a.queries != nil {
		_ = a.queries.ListUsageBySession
	}
	writeJSON(w, http.StatusOK, UsageResponse{})
}

// GetRunUsage 按 Run 查询用量记录。
// TODO: 按 run_id 查询用量记录。
func (a *API) GetRunUsage(w http.ResponseWriter, _ *http.Request) {
	if a.queries != nil {
		_ = a.queries.ListUsageByRun
	}
	writeJSON(w, http.StatusOK, UsageResponse{})
}
