package handler

import "net/http"

// SubscribeEvents 先回放持久化事件，再订阅进程内总线。
// TODO: 按 afterSeq 回放持久化事件，再订阅 Bus 并以 SSE 写出。
func (a *API) SubscribeEvents(w http.ResponseWriter, _ *http.Request) {
	if a.queries != nil {
		_ = a.queries.ListSessionEventsAfter
	}
	_ = a.bus
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
}
