package codex

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	intcodex "codedock/internal/codex"
	cderr "codedock/internal/errors"
	pkg "codedock/pkg/codex"
)

// API 是 /codex HTTP 薄桥接。
type API struct {
	rt *intcodex.Runtime
}

// New 构造 Codex HTTP 入口。
func New(rt *intcodex.Runtime) *API {
	return &API{rt: rt}
}

// Mount 把 /codex 路由挂到父路由器上。
func (a *API) Mount(r chi.Router) {
	if a == nil || a.rt == nil {
		return
	}
	r.Route("/codex", func(r chi.Router) {
		r.Get("/status", a.Status)
		r.Get("/models", a.Models)
		r.Get("/modes", a.Modes)
		r.Get("/commands", a.Commands)
		r.Get("/sessions", a.ListSessions)
		r.Post("/sessions", a.CreateSession)
		r.Get("/sessions/{id}", a.GetSession)
		r.Patch("/sessions/{id}", a.PatchSession)
		r.Post("/sessions/{id}/fork", a.ForkSession)
		r.Post("/sessions/{id}/archive", a.ArchiveSession)
		r.Post("/sessions/{id}/compact", a.CompactSession)
		r.Post("/sessions/{id}/review", a.ReviewSession)
		r.Get("/sessions/{id}/settings", a.GetSettings)
		r.Post("/sessions/{id}/settings", a.ApplySettings)
		r.Post("/sessions/{id}/commands", a.InvokeCommand)
		r.Post("/sessions/{id}/turns", a.StartTurn)
		r.Post("/sessions/{id}/attachments/mention", a.Mention)
		r.Post("/sessions/{id}/attachments/image", a.AttachImage)
		r.Get("/sessions/{id}/events", a.Events)
		r.Get("/sessions/{id}/asks", a.ListAsks)
		r.Post("/turns/{id}/interrupt", a.InterruptTurn)
		r.Post("/asks/{request_id}/decision", a.Decide)
		r.Post("/asks/{request_id}/expire", a.Expire)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case cderr.IsNotFound(err):
		status = http.StatusNotFound
	case cderr.IsConflict(err):
		status = http.StatusConflict
	case cderr.IsInvalid(err):
		status = http.StatusBadRequest
	case cderr.IsUnauthorized(err):
		status = http.StatusUnauthorized
	case cderr.IsUnavailable(err):
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func decodeJSON(r *http.Request, dest any) error {
	if r.Body == nil {
		return nil
	}
	if err := json.NewDecoder(r.Body).Decode(dest); err != nil {
		return cderr.Invalid("%s", err.Error())
	}
	return nil
}

func (a *API) Status(w http.ResponseWriter, r *http.Request) {
	status, err := a.rt.Probe(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (a *API) Models(w http.ResponseWriter, r *http.Request) {
	models, err := a.rt.ListModels(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (a *API) Modes(w http.ResponseWriter, r *http.Request) {
	modes, err := a.rt.ListModes(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"modes": modes})
}

func (a *API) Commands(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"commands": a.rt.Commands()})
}

func (a *API) ListSessions(w http.ResponseWriter, r *http.Request) {
	archived := r.URL.Query().Get("archived") == "true"
	page, err := a.rt.ListSessions(r.Context(), archived, r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

type createSessionRequest struct {
	Settings pkg.Settings `json:"settings"`
}

func (a *API) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	session, err := a.rt.CreateSession(r.Context(), req.Settings)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"session": session})
}

func (a *API) GetSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	session, progress, err := a.rt.GetSession(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"session": session, "progress": progress, "asks": a.rt.PendingAsks(id)})
}

type patchSessionRequest struct {
	Title string `json:"title"`
}

func (a *API) PatchSession(w http.ResponseWriter, r *http.Request) {
	var req patchSessionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := a.rt.Rename(r.Context(), chi.URLParam(r, "id"), req.Title); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (a *API) ForkSession(w http.ResponseWriter, r *http.Request) {
	session, err := a.rt.Fork(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"session": session})
}

func (a *API) ArchiveSession(w http.ResponseWriter, r *http.Request) {
	if err := a.rt.Archive(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (a *API) CompactSession(w http.ResponseWriter, r *http.Request) {
	if err := a.rt.Compact(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (a *API) ReviewSession(w http.ResponseWriter, r *http.Request) {
	if err := a.rt.Review(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (a *API) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := a.rt.Effective(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (a *API) ApplySettings(w http.ResponseWriter, r *http.Request) {
	var patch pkg.Settings
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, err)
		return
	}
	settings, err := a.rt.ApplySettings(r.Context(), chi.URLParam(r, "id"), patch)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

type commandRequest struct {
	Name string `json:"name"`
	Args string `json:"args"`
}

func (a *API) InvokeCommand(w http.ResponseWriter, r *http.Request) {
	var req commandRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	result, err := a.rt.Invoke(r.Context(), chi.URLParam(r, "id"), req.Name, req.Args)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type startTurnRequest struct {
	Content string        `json:"content"`
	Input   pkg.Input     `json:"input"`
	Mode    pkg.InputMode `json:"mode"`
}

func (a *API) StartTurn(w http.ResponseWriter, r *http.Request) {
	var req startTurnRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	mode := req.Mode
	if mode == "" {
		mode = pkg.InputStart
	}
	turn, err := a.rt.StartTurn(r.Context(), chi.URLParam(r, "id"), req.Content, req.Input, mode)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"turn": turn})
}

type pathRequest struct {
	Path string `json:"path"`
}

func (a *API) Mention(w http.ResponseWriter, r *http.Request) {
	var req pathRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := a.rt.Mention(r.Context(), chi.URLParam(r, "id"), req.Path); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (a *API) AttachImage(w http.ResponseWriter, r *http.Request) {
	var req pathRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := a.rt.AttachImage(r.Context(), chi.URLParam(r, "id"), req.Path); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (a *API) ListAsks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"asks": a.rt.PendingAsks(chi.URLParam(r, "id"))})
}

type interruptRequest struct {
	SessionID string `json:"session_id"`
}

func (a *API) InterruptTurn(w http.ResponseWriter, r *http.Request) {
	var req interruptRequest
	_ = decodeJSON(r, &req)
	if req.SessionID == "" {
		writeError(w, cderr.Invalid("session_id is required"))
		return
	}
	if err := a.rt.CancelTurn(r.Context(), req.SessionID, chi.URLParam(r, "id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (a *API) Decide(w http.ResponseWriter, r *http.Request) {
	var answer pkg.AskAnswer
	if err := decodeJSON(r, &answer); err != nil {
		writeError(w, err)
		return
	}
	if err := a.rt.Decide(r.Context(), chi.URLParam(r, "request_id"), answer); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (a *API) Expire(w http.ResponseWriter, r *http.Request) {
	if err := a.rt.Expire(r.Context(), chi.URLParam(r, "request_id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (a *API) Events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, fmt.Errorf("streaming unsupported"))
		return
	}
	sessionID := chi.URLParam(r, "id")
	after := int64(0)
	if raw := r.URL.Query().Get("after"); raw != "" {
		after, _ = strconv.ParseInt(raw, 10, 64)
	}
	if last := r.Header.Get("Last-Event-ID"); last != "" {
		after, _ = strconv.ParseInt(last, 10, 64)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	replay, reset := a.rt.Events(sessionID, after)
	if reset {
		writeSSE(w, pkg.Event{Type: pkg.EventReset, SessionID: sessionID, Notice: "event gap; rehydrate via GET /codex/sessions/{id}"})
		flusher.Flush()
	}
	for _, ev := range replay {
		writeSSE(w, ev)
		flusher.Flush()
	}
	live, unsub := a.rt.Subscribe(sessionID)
	defer unsub()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-live:
			if !ok {
				return
			}
			writeSSE(w, ev)
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, ev pkg.Event) {
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Type, body)
}
