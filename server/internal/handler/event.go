package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"codedock/internal/events"
	pkgagent "codedock/pkg/agent"
	"codedock/pkg/db/sqlite"
)

type ListEventsResponse struct {
	Events []pkgagent.AgentEvent `json:"events"`
}

// ListEvents 按 seq 回放已落库事件，供客户端一次 hydrate。
func (a *API) ListEvents(w http.ResponseWriter, r *http.Request) {
	session, err := a.loadSession(r)
	if err != nil {
		writeError(w, err)
		return
	}
	rows, err := a.q(r.Context()).ListSessionEventsAfter(r.Context(), sqlite.ListSessionEventsAfterParams{
		SessionID: session.ID,
		Seq:       parseAfterSeq(r),
	})
	if err != nil {
		writeError(w, wrapHandlerDB(err))
		return
	}
	events := make([]pkgagent.AgentEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, mapEvent(row))
	}
	writeJSON(w, http.StatusOK, ListEventsResponse{Events: events})
}

// SubscribeEvents 先回放持久化事件，再订阅进程内总线。
// 用 after / Last-Event-ID 定位；直播帧按 seq 去重。客户端断开不取消 Run。
func (a *API) SubscribeEvents(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "session_id")
	afterSeq := parseAfterSeq(r)
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, fmt.Errorf("streaming unsupported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	rows, err := a.q(r.Context()).ListSessionEventsAfter(r.Context(), sqlite.ListSessionEventsAfterParams{
		SessionID: sessionID,
		Seq:       afterSeq,
	})
	if err != nil {
		return
	}
	last := afterSeq
	for _, row := range rows {
		ev := mapEvent(row)
		writeSSE(w, ev)
		flusher.Flush()
		last = ev.Seq
	}

	live := make(chan pkgagent.AgentEvent, 32)
	unsub := a.bus.SubscribeAll(func(e events.Event) {
		if e.ChatSessionID != sessionID {
			return
		}
		ev, ok := e.Payload.(pkgagent.AgentEvent)
		if !ok {
			return
		}
		select {
		case live <- ev:
		default:
		}
	})
	defer unsub()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-live:
			if ev.Seq <= last {
				continue
			}
			writeSSE(w, ev)
			flusher.Flush()
			last = ev.Seq
		}
	}
}

// parseAfterSeq 从 query after 或 Last-Event-ID 读取回放起点。
func parseAfterSeq(r *http.Request) int64 {
	if raw := r.URL.Query().Get("after"); raw != "" {
		n, _ := strconv.ParseInt(raw, 10, 64)
		return n
	}
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		n, _ := strconv.ParseInt(raw, 10, 64)
		return n
	}
	return 0
}

// writeSSE 写出一条 id/event/data 帧。
func writeSSE(w http.ResponseWriter, ev pkgagent.AgentEvent) {
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "id: %d\n", ev.Seq)
	_, _ = fmt.Fprintf(w, "event: %s\n", ev.Type)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", body)
}
