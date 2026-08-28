package handler

import (
	"net/http"

	pkgagent "codedock/pkg/agent"
)

type UsageResponse struct {
	Records []pkgagent.UsageRecord
}

// GetSessionUsage 按会话查询用量记录。本阶段为空实现。
func (a *API) GetSessionUsage(w http.ResponseWriter, _ *http.Request) {
	if a.queries != nil {
		_ = a.queries.ListUsageBySession
	}
	writeJSON(w, http.StatusOK, UsageResponse{})
}

// GetRunUsage 按 Run 查询用量记录。本阶段为空实现。
func (a *API) GetRunUsage(w http.ResponseWriter, _ *http.Request) {
	if a.queries != nil {
		_ = a.queries.ListUsageByRun
	}
	writeJSON(w, http.StatusOK, UsageResponse{})
}
