package handler

import "net/http"

// SubscribeEvents 先回放持久化事件，再订阅进程内总线。本阶段为空实现。
func (a *API) SubscribeEvents(w http.ResponseWriter, _ *http.Request) {
	if a.queries != nil {
		_ = a.queries.ListSessionEventsAfter
	}
	_ = a.bus
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
}
